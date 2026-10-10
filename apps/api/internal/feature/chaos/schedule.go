package chaos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
	writechaos "github.com/shimaiku1960/juken-map/apps/api/internal/write/chaos"
)

// 予告なしで実験を始めるくじ（JUK-176）。CHAOS_ENABLED=on に加えて CHAOS_SCHEDULE=on のときだけ、API のプロセスの中で動く。
//
// GitHub Actions の schedule は毎回2〜4時間遅れる（毎日の通知で踏んだ、infra/systemd/）ので、時間帯を守れるようにここで動かす。
// 外から呼ばないので、共有トークン（CHAOS_SECRET）も要らない。
//
//   - 平日の 10〜22 時（日本時間）だけ。終わる時刻も 22 時を越えない。祝日は区別しない
//   - drawInterval ごとにくじを引き、当たりが週に2〜3回になるようにする
//   - 起動してから startupGrace の間は引かない。デプロイの直後（新しいコンテナ）に起こさないため。デプロイの前後に
//     実行中の実験を止めるのは .github/scripts/deploy-ec2.sh（`/api chaos stop`）
//   - 予告しない。終わったら、何を起こしたかを運営者（ADMIN_NOTIFICATION_EMAIL）へメールで知らせる
const (
	drawInterval = 10 * time.Minute
	startupGrace = 15 * time.Minute
	// windowStartHour・windowEndHour は起こしてよい時間帯（日本時間）。
	windowStartHour = 10
	windowEndHour   = 22
	// drawsPerWeek は平日 10〜22 時に引くくじの回数（5日 × 12時間 × 6回）。当たりの平均を週 targetPerWeek 回にする。
	drawsPerWeek  = 5 * (windowEndHour - windowStartHour) * int(time.Hour/drawInterval)
	targetPerWeek = 2.5
	// watchInterval は始めた実験が終わったかを見る間隔。デプロイや管理画面で止めたときも、これで気づいて知らせる。
	watchInterval = 30 * time.Second
)

// 当たったときの中身の候補。どれも fault.Spec.Validate を通る値にする（schedule_test.go）。
var (
	drawDurations    = []time.Duration{10 * time.Minute, 15 * time.Minute, 20 * time.Minute, 30 * time.Minute}
	drawRates        = []float64{0.1, 0.25, 0.5, 1}
	drawLatencyMs    = []int{1500, 3000, 6000}
	drawTimeoutMs    = []int{3000, 6000, 9000}
	drawAllRoutesPct = 0.5
)

// Scheduler は予告なしの実験を始め、終わったら知らせる。
type Scheduler struct {
	DB       *sql.DB
	Injector *fault.Injector
	// Routes は障害注入の対象にしてよいルート（httpx.Router.FaultRoutes）。
	Routes func() []string
	// Notify は運営者へメールを送る。宛先が無ければ nil にし、ログに残すだけにする。
	Notify func(ctx context.Context, subject, body string) error
	Rand   *rand.Rand
	Now    func() time.Time // DATETIME(3) に書くので、ミリ秒で切り捨てた時刻
	// watchEvery は始めた実験が終わったかを見る間隔（本番は watchInterval）。
	watchEvery time.Duration
}

// NewScheduler は本番の Scheduler を作る。
func NewScheduler(db *sql.DB, injector *fault.Injector, routes func() []string,
	notify func(ctx context.Context, subject, body string) error) *Scheduler {
	// #nosec G404 -- 練習の障害を選ぶくじで、秘密や認証には使わない
	r := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	return &Scheduler{DB: db, Injector: injector, Routes: routes, Notify: notify, Rand: r, Now: dates.NowMillis,
		watchEvery: watchInterval}
}

// Run は ctx が終わるまでくじを引き続ける。
func (s *Scheduler) Run(ctx context.Context) {
	if sleep(ctx, startupGrace) != nil {
		return
	}
	for {
		if spec, ok := Draw(s.Rand, s.Now(), s.Routes()); ok {
			s.run(ctx, spec)
		}
		if sleep(ctx, drawInterval) != nil {
			return
		}
	}
}

// run は1つの実験を始め、終わるまで待ってから知らせる。始められなかったとき（ほかの実験が実行中など）は何もしない。
func (s *Scheduler) run(ctx context.Context, spec fault.Spec) {
	now := s.Now()
	id, err := writechaos.Start(ctx, s.DB, writechaos.Experiment{
		Kind:       string(spec.Kind),
		Route:      spec.Route,
		Rate:       spec.Rate,
		DelayMs:    spec.DelayMs,
		StatusCode: spec.StatusCode,
		StartsAt:   now,
		EndsAt:     now.Add(spec.Duration),
	}, now)
	if errors.Is(err, writechaos.ErrRunning) {
		return
	}
	if err != nil {
		// 実験があることをログで知らせないよう、何の処理かは書かない（JUK-178）。
		slog.Warn("[schedule] Failed to start.", "err", err.Error())
		return
	}
	if err := s.Injector.Refresh(ctx, s.DB); err != nil {
		slog.Warn("[schedule] Failed to refresh.", "err", err.Error())
	}
	rec, err := s.wait(ctx, id)
	if err != nil {
		return
	}
	s.notify(ctx, rec)
}

// wait は id の実験が終わる（終わる時刻が来た・止めた）まで待ち、終わった記録を返す。ctx が終われば ctx.Err()。
func (s *Scheduler) wait(ctx context.Context, id int64) (fault.Record, error) {
	for {
		if err := sleep(ctx, s.watchEvery); err != nil {
			return fault.Record{}, err
		}
		records, err := fault.Finished(ctx, s.DB, s.Now(), recentLimit)
		if err != nil {
			continue
		}
		for _, rec := range records {
			if rec.ID == id {
				return rec, nil
			}
		}
	}
}

func (s *Scheduler) notify(ctx context.Context, rec fault.Record) {
	if s.Notify == nil {
		slog.Warn("[schedule] ADMIN_NOTIFICATION_EMAIL is not configured.")
		return
	}
	subject, body := reportEmail(rec)
	if err := s.Notify(ctx, subject, body); err != nil {
		slog.Warn("[schedule] Failed to notify.", "err", err.Error())
	}
}

// Draw は now にくじを1回引き、当たれば起こす実験を返す。routes は障害注入の対象にしてよいルート。
func Draw(r *rand.Rand, now time.Time, routes []string) (fault.Spec, bool) {
	if r.Float64() >= targetPerWeek/float64(drawsPerWeek) {
		return fault.Spec{}, false
	}
	spec := fault.Spec{
		Kind:     fault.Kinds[r.IntN(len(fault.Kinds))],
		Route:    fault.AllRoutes,
		Rate:     drawRates[r.IntN(len(drawRates))],
		Duration: drawDurations[r.IntN(len(drawDurations))],
	}
	if !inWindow(now, spec.Duration) {
		return fault.Spec{}, false
	}
	// 外部 API を呼ぶルートは少ないので、外部 API のタイムアウトは全ルートにする（1つのルートでは起きないことが多い）。
	if spec.Kind != fault.KindOutboundTimeout && len(routes) > 0 && r.Float64() >= drawAllRoutesPct {
		spec.Route = routes[r.IntN(len(routes))]
	}
	switch spec.Kind {
	case fault.KindLatency:
		spec.DelayMs = drawLatencyMs[r.IntN(len(drawLatencyMs))]
	case fault.KindOutboundTimeout:
		spec.DelayMs = drawTimeoutMs[r.IntN(len(drawTimeoutMs))]
	case fault.KindHTTPError:
		spec.StatusCode = fault.HTTPErrorStatuses[r.IntN(len(fault.HTTPErrorStatuses))]
	}
	return spec, true
}

// inWindow は、now に始めて d 続く実験が、平日の 10〜22 時（日本時間）に収まるか。
func inWindow(now time.Time, d time.Duration) bool {
	t := now.In(dates.Tokyo)
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	start := time.Date(t.Year(), t.Month(), t.Day(), windowStartHour, 0, 0, 0, dates.Tokyo)
	end := time.Date(t.Year(), t.Month(), t.Day(), windowEndHour, 0, 0, 0, dates.Tokyo)
	return !t.Before(start) && !t.Add(d).After(end)
}

// reportEmail は終わった実験を知らせるメールの件名と本文。
// 気づくまでの時間（MTTD）と直るまでの時間（MTTR）を振り返りの記録で出せるよう、始めた・終わった時刻と終わり方を書く。
func reportEmail(rec fault.Record) (string, string) {
	format := func(t time.Time) string { return t.In(dates.Tokyo).Format("2006年1月2日 15:04:05") }
	endedAt, endedHow := rec.EndsAt, "終わる時刻が来て戻った"
	if rec.StoppedAt != nil {
		endedAt, endedHow = *rec.StoppedAt, stoppedByLabel(rec.StoppedBy)+"で止めた"
	}
	route := rec.Route
	if route == fault.AllRoutes {
		route = "すべて（* ）"
	}
	detail := "—"
	switch {
	case rec.Kind == fault.KindHTTPError:
		detail = fmt.Sprintf("%d を返す", rec.StatusCode)
	case rec.DelayMs > 0:
		detail = fmt.Sprintf("%dms", rec.DelayMs)
	}
	row := func(k, v string) string { return "<dt>" + k + "</dt><dd>" + html.EscapeString(v) + "</dd>" }
	body := "<p>予告なしの障害注入の実験が終わりました。振り返りの記録を書いてください。</p><dl>" +
		row("始めた", format(rec.StartsAt)) +
		row("終わった", format(endedAt)+"（"+endedHow+"）") +
		row("種類", kindLabel(rec.Kind)) +
		row("ルート", route) +
		row("割合", fmt.Sprintf("%.0f%%", rec.Rate*100)) +
		row("中身", detail) +
		"</dl><p>終わった実験は管理者ページ（/admin）の「障害注入」でも見られます。</p>"
	return "【受験マップ】障害注入の実験が終わりました", body
}

func kindLabel(k fault.Kind) string {
	switch k {
	case fault.KindLatency:
		return "遅延"
	case fault.KindHTTPError:
		return "5xx"
	case fault.KindDBError:
		return "DB の失敗"
	case fault.KindOutboundTimeout:
		return "外部 API のタイムアウト"
	}
	return string(k)
}

// stoppedByLabel は stoppedBy（admin:<userId>・deploy・job）を、知らせる文の言葉にする。
func stoppedByLabel(by string) string {
	switch by {
	case writechaos.StoppedByDeploy:
		return "デプロイ"
	case writechaos.StoppedByJob:
		return "機械の入口（/api/chaos/）"
	}
	return "管理画面"
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
