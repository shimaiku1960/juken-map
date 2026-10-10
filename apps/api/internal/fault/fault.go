// Package fault は本番で障害を起こす（カオスエンジニアリング、JUK-171）。段階1（JUK-173）はアプリの層で、
// HTTP の遅延・5xx、DB の失敗、外部 API のタイムアウトを起こす。
//
// 実験（ChaosExperiment の行）は機械の入口（/api/chaos/、段階4のスケジューラー）が作り、管理画面（/admin）からも止められる。
// Injector は実行中の実験を DB から5秒ごとに読み、ルーター（http.go）・DB の接続（db.go）・外部 API のクライアント（outbound.go）の
// 3か所で使う。
//
// 安全のための決まり:
//   - 起こすのは、ルーターが対象にしてよいと印を付けたリクエスト（telemetry.RequestInfo.FaultRoute）の中だけ。
//     管理画面・ログイン・障害注入の入口と、リクエストの外の処理（このパッケージの読み込みを含む）には起こさない
//   - 終わる時刻はメモリの上でも確かめる。DB が読めなくなっても、時刻が来れば止まる
//   - CHAOS_ENABLED を消せば Injector を作らず（nil）、何も起こさない。nil の Injector のメソッドは何もしない
//
// 練習のための決まり（JUK-178）: 予告なしの障害で原因を調べる練習をするので、起こしたことはログ・スパン・数値の
// どこにも出さず、誤りの文言も本物と同じにする。何をいつ起こしたかは ChaosExperiment の行が答え合わせになる。
package fault

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// Kind は起こす障害の種類。
type Kind string

const (
	KindLatency         Kind = "latency"          // 応答を DelayMs 遅らせてからハンドラへ進む
	KindHTTPError       Kind = "http_error"       // ハンドラへ進まず StatusCode を返す
	KindDBError         Kind = "db_error"         // SQL を流さずに失敗させる
	KindOutboundTimeout Kind = "outbound_timeout" // 外部 API を DelayMs 待たせてから、タイムアウトで失敗させる
)

// Kinds は起こせる障害の種類の全部。
var Kinds = []Kind{KindLatency, KindHTTPError, KindDBError, KindOutboundTimeout}

// AllRoutes を Route に入れると、/api/ の対象のルートのうち /api/health を除く全部に起こす。
// /api/health は外からの死活監視とデプロイ後の確認が叩くので、名指ししたときだけ対象にする。
const AllRoutes = "*"

// ErrInjected は、障害注入で失敗させた DB の操作の誤り。調べる練習で答えにならないよう、接続が切れたときの
// go-sql-driver/mysql の誤り（mysql.ErrInvalidConn）と同じ文言にする（JUK-178）。
var ErrInjected = errors.New("invalid connection")

// pollInterval は実行中の実験を DB から読み直す間隔。管理画面で止めてから効くまで、最大でこの時間かかる
// （同じプロセスで始めた・止めたものは、その場で読み直す）。
const pollInterval = 5 * time.Second

// Experiment は実行中の実験1つ。
type Experiment struct {
	ID         int64
	Kind       Kind
	Route      string  // AllRoutes か「GET /api/study-logs/:id」の形
	Rate       float64 // 対象のリクエストのうち障害を起こす割合（0 より大きく 1 以下）
	DelayMs    int     // latency・outbound_timeout の待ち時間
	StatusCode int     // http_error で返すステータス
	StartsAt   time.Time
	EndsAt     time.Time
}

func (e Experiment) runningAt(now time.Time) bool {
	return !now.Before(e.StartsAt) && now.Before(e.EndsAt)
}

func (e Experiment) matches(route string) bool {
	if e.Route != AllRoutes {
		return e.Route == route
	}
	_, path, _ := strings.Cut(route, " ")
	return strings.HasPrefix(path, "/api/") && path != "/api/health"
}

func (e Experiment) delay() time.Duration {
	return time.Duration(e.DelayMs) * time.Millisecond
}

// Injector は実行中の実験を持ち、リクエストごとに障害を起こすかを決める。
type Injector struct {
	now   func() time.Time
	roll  func() float64 // [0, 1) の乱数
	sleep func(ctx context.Context, d time.Duration) error

	active atomic.Pointer[[]Experiment]
}

// New は Injector を作る。障害を起こしたかどうかは数値にもログにも出さない。調べる練習で答えにならないようにするため
// （JUK-178）。何をいつ起こしたかは ChaosExperiment の行が答え合わせになる。
func New() *Injector {
	in := &Injector{
		now: time.Now,
		// #nosec G404 -- 障害を起こすかどうかのくじ引き。予測されても困らない
		roll:  rand.Float64,
		sleep: sleepContext,
	}
	in.active.Store(&[]Experiment{})
	return in
}

// pick は ctx のリクエストに kind の障害を起こすなら、その実験を返す。
func (in *Injector) pick(ctx context.Context, kind Kind) (Experiment, bool) {
	if in == nil {
		return Experiment{}, false
	}
	info := telemetry.RequestInfoFrom(ctx)
	if info == nil || info.FaultRoute == "" {
		return Experiment{}, false
	}
	now := in.now()
	for _, e := range *in.active.Load() {
		if e.Kind != kind || !e.runningAt(now) || !e.matches(info.FaultRoute) {
			continue
		}
		if in.roll() >= e.Rate {
			return Experiment{}, false
		}
		return e, true
	}
	return Experiment{}, false
}

// Run は起動したときと、その後 pollInterval ごとに実行中の実験を読み直す。ctx が取り消されたら戻る。
func (in *Injector) Run(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if err := in.Refresh(ctx, db); err != nil && ctx.Err() == nil {
			// 前に読んだ実験はそのまま使う。終わる時刻はメモリの上で確かめるので、止まらなくなることはない。
			slog.Warn("[chaos] Failed to load experiments.", "err", err.Error())
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Refresh は実行中の実験を DB から読み直す。始まった・終わったことはログに残さない（答えになるため）。
func (in *Injector) Refresh(ctx context.Context, db *sql.DB) error {
	if in == nil {
		return nil
	}
	list, err := Running(ctx, db, in.now())
	if err != nil {
		return err
	}
	in.active.Store(&list)
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
