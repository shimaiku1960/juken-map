package chaos

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 入力の決まり。機械の入口の本文の形は、ほかの機能（validation.Issue）と違い、誤りの文だけを返す（{"error": "…"}）。

// InputError は、始めようとした実験が決まりに合わないこと。Error() は利用者に見せる文言。
type InputError string

func (e InputError) Error() string { return string(e) }

func inputErrorf(format string, args ...any) InputError {
	return InputError(fmt.Sprintf(format, args...))
}

// 実験の上限。
const (
	// MaxDuration は1つの実験の長さの上限。終わる時刻を必ず決め、長く続けない。
	MaxDuration = time.Hour
	MinDuration = time.Minute
	// MaxDelayMs は待たせる時間の上限。1リクエストの時間の上限（internal/app/server.go の requestTimeout、10秒）より短くし、
	// 待ったあとに応答を返せるようにする。
	MaxDelayMs = 9000
)

// HTTPErrorStatuses は http_error で返せるステータス。
var HTTPErrorStatuses = []int{500, 502, 503, 504}

// Spec は新しく始める実験の中身。
type Spec struct {
	Kind       Kind
	Route      string
	Rate       float64
	DelayMs    int
	StatusCode int
	Duration   time.Duration
}

// Validate は Spec が起こしてよい実験かを確かめる。routes は障害注入の対象にしてよいルート（httpx.Router.FaultRoutes）。
// 決まりに合わなければ InputError（文はコントローラーがそのまま 400 で返す）。
func (s Spec) Validate(routes []string) error {
	if !slices.Contains(Kinds, s.Kind) {
		return InputError("kind が正しくありません")
	}
	if s.Route != AllRoutes && !slices.Contains(routes, s.Route) {
		return InputError("route は * か、障害を起こしてよいルート（「GET /api/study-logs/:id」の形）にしてください")
	}
	if !(s.Rate > 0 && s.Rate <= 1) {
		return InputError("rate は 0 より大きく 1 以下にしてください")
	}
	if s.Duration < MinDuration || s.Duration > MaxDuration {
		return inputErrorf("durationSeconds は %d〜%d にしてください", int(MinDuration.Seconds()), int(MaxDuration.Seconds()))
	}
	switch s.Kind {
	case KindLatency, KindOutboundTimeout:
		if s.DelayMs < 1 || s.DelayMs > MaxDelayMs {
			return inputErrorf("delayMs は 1〜%d にしてください", MaxDelayMs)
		}
		if s.StatusCode != 0 {
			return InputError("statusCode は http_error のときだけ指定できます")
		}
	case KindHTTPError:
		if !slices.Contains(HTTPErrorStatuses, s.StatusCode) {
			return InputError("statusCode は 500・502・503・504 のどれかにしてください")
		}
		if s.DelayMs != 0 {
			return InputError("delayMs は latency・outbound_timeout のときだけ指定できます")
		}
	case KindDBError:
		if s.DelayMs != 0 || s.StatusCode != 0 {
			return InputError("db_error には delayMs・statusCode を指定できません")
		}
	}
	return nil
}

// StartingAt は Spec を now から始める実験にする。startedBy は始めた入口（StartedBy…）。
func (s Spec) StartingAt(now time.Time, startedBy string) NewExperiment {
	return NewExperiment{
		Kind:       s.Kind,
		Route:      s.Route,
		Rate:       s.Rate,
		DelayMs:    s.DelayMs,
		StatusCode: s.StatusCode,
		StartsAt:   now,
		EndsAt:     now.Add(s.Duration),
		StartedBy:  startedBy,
	}
}

// decodeSpec は POST /api/chaos/experiments の本文を読む。余計なキーは無視する。値の範囲は Validate が確かめる。
func decodeSpec(body any) (Spec, error) {
	m, ok := body.(map[string]any)
	if !ok {
		return Spec{}, InputError("本文は JSON のオブジェクトにしてください")
	}
	kind, _ := m["kind"].(string)
	route, _ := m["route"].(string)
	rate, ok := number(m["rate"])
	if !ok {
		return Spec{}, InputError("rate は数にしてください")
	}
	seconds, ok := integer(m["durationSeconds"])
	if !ok {
		return Spec{}, InputError("durationSeconds は整数にしてください")
	}
	spec := Spec{Kind: Kind(kind), Route: route, Rate: rate, Duration: time.Duration(seconds) * time.Second}
	for _, f := range []struct {
		key string
		dst *int
	}{{"delayMs", &spec.DelayMs}, {"statusCode", &spec.StatusCode}} {
		v, present := m[f.key]
		if !present {
			continue
		}
		n, ok := integer(v)
		if !ok {
			return Spec{}, inputErrorf("%s は整数にしてください", f.key)
		}
		*f.dst = int(n)
	}
	return spec, nil
}

func number(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

// integer は小数を含まない数（1.0 は整数）。範囲は呼び出し側が確かめるので、ここでは int に収まるかだけを見る。
func integer(v any) (int64, bool) {
	f, ok := number(v)
	if !ok || f != math.Trunc(f) || math.Abs(f) > validation.MaxSafeInteger {
		return 0, false
	}
	return int64(f), true
}
