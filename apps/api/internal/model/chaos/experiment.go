package chaos

import (
	"strings"
	"time"
)

// 実験の種類と中身。

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

// Experiment は記録した実験1つ（始める時刻・終わる時刻を含む）。
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

// RunningAt は now の時点で実行中か（止めたかどうかは見ない。止めた実験は読み込みで除く）。
func (e Experiment) RunningAt(now time.Time) bool {
	return !now.Before(e.StartsAt) && now.Before(e.EndsAt)
}

// Matches は route（「GET /api/study-logs/:id」の形）のリクエストが対象か。
func (e Experiment) Matches(route string) bool {
	if e.Route != AllRoutes {
		return e.Route == route
	}
	_, path, _ := strings.Cut(route, " ")
	return strings.HasPrefix(path, "/api/") && path != "/api/health"
}

// Delay は latency・outbound_timeout の待ち時間。
func (e Experiment) Delay() time.Duration {
	return time.Duration(e.DelayMs) * time.Millisecond
}

// Record は記録に残った実験1つ。
type Record struct {
	Experiment
	StoppedAt *time.Time
	StoppedBy string
	StartedBy string // 始めた入口（job・schedule）
}
