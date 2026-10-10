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
	"github.com/shimaiku1960/juken-map/apps/api/internal/hostfault"
	writechaos "github.com/shimaiku1960/juken-map/apps/api/internal/write/chaos"
)

// 予告なしで実験を始めるくじ（JUK-176）。CHAOS_ENABLED=on に加えて CHAOS_SCHEDULE=on のときだけ、API のプロセスの中で動く。
// CHAOS_HOST=on なら、ホストの層の障害（internal/hostfault、JUK-174）も半分の割合で選ぶ。
//
// GitHub Actions の schedule は毎回2〜4時間遅れる（毎日の通知で踏んだ、infra/systemd/）ので、時間帯を守れるようにここで動かす。
// 外から呼ばないので、共有トークン（CHAOS_SECRET）も要らない。
//
//   - 平日の 10〜22 時（日本時間）だけ。終わる時刻も 22 時を越えない。祝日は区別しない
//   - drawInterval ごとにくじを引き、当たりが週に2〜3回になるようにする
//   - 起動してから startupGrace の間は引かない。デプロイの直後（新しいコンテナ）に起こさないため。デプロイの前後に
//     実行中の実験を止めるのは .github/scripts/deploy-ec2.sh（`/api chaos stop`）
//   - 予告しない。終わったら、何を起こしたかを運営者（ADMIN_NOTIFICATION_EMAIL）へメールで知らせる。ホストの障害では
//     始めたプロセスが途中で落ちることがあるので、終わるまで待たず、どのプロセスでも watchInterval ごとに
//     「終わったのにまだ知らせていない」実験（fault.Unreported）を拾って知らせる
const (
	drawInterval = 10 * time.Minute
	startupGrace = 15 * time.Minute
	// windowStartHour・windowEndHour は起こしてよい時間帯（日本時間）。
	windowStartHour = 10
	windowEndHour   = 22
	// drawsPerWeek は平日 10〜22 時に引くくじの回数（5日 × 12時間 × 6回）。当たりの平均を週 targetPerWeek 回にする。
	drawsPerWeek  = 5 * (windowEndHour - windowStartHour) * int(time.Hour/drawInterval)
	targetPerWeek = 2.5
	// watchInterval は終わった実験を探す間隔。デプロイや管理画面で止めたときも、これで気づいて知らせる。
	watchInterval = 30 * time.Second
	// hostPct はホストの層の障害を選ぶ割合（CHAOS_HOST=on のとき）。
	hostPct = 0.5
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
	// Host はホストの層の障害を起こす（CHAOS_HOST=on のときだけ。nil ならアプリの層だけ）。
	Host hostfault.Runner
	Rand *rand.Rand
	Now  func() time.Time // DATETIME(3) に書くので、ミリ秒で切り捨てた時刻
	// watchEvery は終わった実験を探す間隔（本番は watchInterval）。
	watchEvery time.Duration
}

// NewScheduler は本番の Scheduler を作る。
func NewScheduler(db *sql.DB, injector *fault.Injector, routes func() []string,
	notify func(ctx context.Context, subject, body string) error, host hostfault.Runner) *Scheduler {
	// #nosec G404 -- 練習の障害を選ぶくじで、秘密や認証には使わない
	r := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	return &Scheduler{DB: db, Injector: injector, Routes: routes, Notify: notify, Host: host, Rand: r,
		Now: dates.NowMillis, watchEvery: watchInterval}
}

// Run は ctx が終わるまで、watchEvery ごとに終わった実験を知らせ、drawInterval ごとにくじを引く。
// 知らせるのは起動してすぐから、くじは startupGrace のあとから。
func (s *Scheduler) Run(ctx context.Context) {
	nextDraw := s.Now().Add(startupGrace)
	for {
		s.report(ctx)
		if now := s.Now(); !now.Before(nextDraw) {
			if spec, ok := Draw(s.Rand, now, s.Routes(), s.Host != nil); ok {
				s.run(ctx, spec)
			}
			nextDraw = now.Add(drawInterval)
		}
		if sleep(ctx, s.watchEvery) != nil {
			return
		}
	}
}

// run は1つの実験を始める。始められなかったとき（ほかの実験が実行中など）は何もしない。
// ホストの障害を起こせなかったら、記録した実験をすぐ止め（stoppedBy=failed）、次の report で知らせる。
func (s *Scheduler) run(ctx context.Context, spec fault.Spec) {
	now := s.Now()
	e := writechaos.Experiment{
		Kind:       string(spec.Kind),
		Route:      spec.Route,
		Rate:       spec.Rate,
		DelayMs:    spec.DelayMs,
		StatusCode: spec.StatusCode,
		StartsAt:   now,
		EndsAt:     now.Add(spec.Duration),
		StartedBy:  writechaos.StartedBySchedule,
	}
	id, err := writechaos.Start(ctx, s.DB, e, now)
	if errors.Is(err, writechaos.ErrRunning) {
		return
	}
	if err != nil {
		// 実験があることをログで知らせないよう、何の処理かは書かない（JUK-178）。
		slog.Warn("[schedule] Failed to start.", "err", err.Error())
		return
	}
	if !hostfault.IsHost(spec.Kind) {
		if err := s.Injector.Refresh(ctx, s.DB); err != nil {
			slog.Warn("[schedule] Failed to refresh.", "err", err.Error())
		}
		return
	}
	err = s.Host.Start(ctx, fault.Experiment{ID: id, Kind: spec.Kind, Rate: e.Rate, DelayMs: e.DelayMs,
		StartsAt: e.StartsAt, EndsAt: e.EndsAt})
	if err == nil {
		return
	}
	slog.Warn("[schedule] Failed to start.", "err", err.Error())
	if err := writechaos.Stop(ctx, s.DB, id, writechaos.StoppedByFailed, s.Now()); err != nil {
		slog.Warn("[schedule] Failed to stop.", "err", err.Error())
	}
}

// report は、くじで始めて終わった実験のうち、まだ知らせていないものを知らせる。ホストの障害を途中で止めた
// （管理画面・デプロイ）ものは、ホストでも戻してから知らせる。
func (s *Scheduler) report(ctx context.Context) {
	records, err := fault.Unreported(ctx, s.DB, s.Now())
	if err != nil {
		slog.Warn("[schedule] Failed to read.", "err", err.Error())
		return
	}
	for _, rec := range records {
		claimed, err := writechaos.ClaimNotify(ctx, s.DB, rec.ID, s.Now())
		if err != nil || !claimed {
			continue
		}
		if hostfault.IsHost(rec.Kind) && rec.StoppedAt != nil && rec.StoppedBy != writechaos.StoppedByFailed && s.Host != nil {
			if err := s.Host.Revert(ctx); err != nil {
				slog.Warn("[schedule] Failed to revert.", "err", err.Error())
			}
		}
		s.notify(ctx, rec)
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
// host なら hostPct の割合でホストの層の障害を選ぶ。
func Draw(r *rand.Rand, now time.Time, routes []string, host bool) (fault.Spec, bool) {
	if r.Float64() >= targetPerWeek/float64(drawsPerWeek) {
		return fault.Spec{}, false
	}
	if host && r.Float64() < hostPct {
		kind := hostfault.Kinds[r.IntN(len(hostfault.Kinds))]
		levels := hostfault.Levels[kind]
		d := drawDurations[r.IntN(len(drawDurations))]
		if kind == hostfault.KindProcessKill {
			d = hostfault.ProcessKillDuration
		}
		if !inWindow(now, d) {
			return fault.Spec{}, false
		}
		return hostfault.Spec(kind, levels[r.IntN(len(levels))], d), true
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
	switch {
	case rec.StoppedAt != nil && rec.StoppedBy == writechaos.StoppedByFailed:
		endedAt, endedHow = *rec.StoppedAt, "ホストで起こせず、すぐ止めた。API のログを見る"
	case rec.StoppedAt != nil:
		endedAt, endedHow = *rec.StoppedAt, stoppedByLabel(rec.StoppedBy)+"で止めた"
	}
	route := rec.Route
	switch {
	case hostfault.IsHost(rec.Kind):
		route = "—（ホスト）"
	case route == fault.AllRoutes:
		route = "すべて（* ）"
	}
	detail := detailLabel(rec.Experiment)
	row := func(k, v string) string { return "<dt>" + k + "</dt><dd>" + html.EscapeString(v) + "</dd>" }
	body := "<p>予告なしの障害注入の実験が終わりました。振り返りの記録を書いてください。</p><dl>" +
		row("始めた", format(rec.StartsAt)) +
		row("終わった", format(endedAt)+"（"+endedHow+"）") +
		row("種類", kindLabel(rec.Kind)) +
		row("ルート", route) +
		row("割合・強さ", fmt.Sprintf("%.0f%%", rec.Rate*100)) +
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
	case hostfault.KindProcessKill:
		return "API のプロセスの kill"
	case hostfault.KindCPU:
		return "CPU の圧迫"
	case hostfault.KindMemory:
		return "メモリの圧迫"
	case hostfault.KindDisk:
		return "ディスクを埋める"
	case hostfault.KindDBDelay:
		return "RDS までの通信の遅延"
	case hostfault.KindDBLoss:
		return "RDS までの通信の喪失"
	}
	return string(k)
}

// detailLabel は実験の中身を、知らせる文の言葉にする。
func detailLabel(e fault.Experiment) string {
	switch e.Kind {
	case fault.KindHTTPError:
		return fmt.Sprintf("%d を返す", e.StatusCode)
	case fault.KindLatency, fault.KindOutboundTimeout:
		return fmt.Sprintf("%dms", e.DelayMs)
	case hostfault.KindProcessKill:
		return "始めに1回 kill -9"
	case hostfault.KindCPU:
		return fmt.Sprintf("全コアを %d%% 使う", hostfault.Level(e))
	case hostfault.KindMemory:
		return fmt.Sprintf("使えるメモリの %d%% を使う", hostfault.Level(e))
	case hostfault.KindDisk:
		return fmt.Sprintf("使用率 %d%% まで埋める", hostfault.Level(e))
	case hostfault.KindDBDelay:
		return fmt.Sprintf("%dms 遅らせる", hostfault.Level(e))
	case hostfault.KindDBLoss:
		return fmt.Sprintf("%d%% 落とす", hostfault.Level(e))
	}
	return "—"
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
