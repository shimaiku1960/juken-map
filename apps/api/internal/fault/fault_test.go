package fault

import (
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/chaos"

	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

var t0 = time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)

// newInjector は、時刻・くじ・待ち時間を固定した Injector に experiments を載せる。待った時間は slept に足す。
func newInjector(t *testing.T, roll float64, experiments ...chaos.Experiment) (*Injector, *time.Duration) {
	t.Helper()
	in := New()
	in.now = func() time.Time { return t0 }
	in.roll = func() float64 { return roll }
	var slept time.Duration
	in.sleep = func(_ context.Context, d time.Duration) error { slept += d; return nil }
	in.active.Store(&experiments)
	return in, &slept
}

func running(kind chaos.Kind, route string) chaos.Experiment {
	return chaos.Experiment{ID: 1, Kind: kind, Route: route, Rate: 0.5, DelayMs: 2000, StatusCode: 503,
		StartsAt: t0.Add(-time.Minute), EndsAt: t0.Add(time.Minute)}
}

func requestCtx(route string) (context.Context, *telemetry.RequestInfo) {
	info := &telemetry.RequestInfo{FaultRoute: route}
	return telemetry.WithRequestInfo(context.Background(), info), info
}

func TestPick(t *testing.T) {
	const route = "GET /api/study-logs"
	tests := []struct {
		name  string
		e     chaos.Experiment
		ctx   string // リクエストの FaultRoute。空なら対象外のルート
		roll  float64
		want  bool
		reach bool // ctx にリクエストの情報があるか（無ければ、リクエストの外の処理）
	}{
		{name: "対象のルートでくじに当たれば起こす", e: running(chaos.KindDBError, route), ctx: route, roll: 0.49, want: true, reach: true},
		{name: "くじに外れれば起こさない", e: running(chaos.KindDBError, route), ctx: route, roll: 0.5, reach: true},
		{name: "ルートが違えば起こさない", e: running(chaos.KindDBError, route), ctx: "GET /api/goals", roll: 0, reach: true},
		{name: "* は /api/ のルート全部に起こす", e: running(chaos.KindDBError, chaos.AllRoutes), ctx: "GET /api/goals", roll: 0, want: true, reach: true},
		{name: "* でも /api/health には起こさない", e: running(chaos.KindDBError, chaos.AllRoutes), ctx: "GET /api/health", roll: 0, reach: true},
		{name: "/api/health は名指しすれば起こす", e: running(chaos.KindDBError, "GET /api/health"), ctx: "GET /api/health", roll: 0, want: true, reach: true},
		{name: "対象外のルート（管理画面・ログイン）には起こさない", e: running(chaos.KindDBError, chaos.AllRoutes), roll: 0, reach: true},
		{name: "リクエストの外の処理には起こさない", e: running(chaos.KindDBError, chaos.AllRoutes), roll: 0},
		{name: "種類が違えば起こさない", e: running(chaos.KindLatency, route), ctx: route, roll: 0, reach: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, _ := newInjector(t, tt.roll, tt.e)
			ctx := context.Background()
			if tt.reach {
				ctx, _ = requestCtx(tt.ctx)
			}

			_, got := in.pick(ctx, chaos.KindDBError)

			if got != tt.want {
				t.Fatalf("pick = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("終わる時刻を過ぎたら、DB から読み直す前でも起こさない", func(t *testing.T) {
		e := running(chaos.KindDBError, chaos.AllRoutes)
		in, _ := newInjector(t, 0, e)
		in.now = func() time.Time { return e.EndsAt }
		ctx, _ := requestCtx(route)

		if _, got := in.pick(ctx, chaos.KindDBError); got {
			t.Fatal("終わった実験で障害を起こした")
		}
	})

	t.Run("Injector が nil（無効）なら何もしない", func(t *testing.T) {
		var in *Injector
		ctx, _ := requestCtx(route)
		if _, got := in.pick(ctx, chaos.KindDBError); got {
			t.Fatal("nil の Injector が障害を起こした")
		}
		if err := in.Refresh(ctx, nil); err != nil {
			t.Fatal(err)
		}
		base := http.DefaultTransport
		if in.Transport(base) != base {
			t.Error("nil の Injector が Transport を包んだ")
		}
	})
}

func TestInjectHTTP(t *testing.T) {
	const route = "POST /api/study-logs"
	serve := func(in *Injector) (*httptest.ResponseRecorder, bool) {
		ctx, _ := requestCtx(route)
		r := httptest.NewRequest(http.MethodPost, "/api/study-logs", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		return w, in.InjectHTTP(w, r)
	}

	t.Run("latency は待ってからハンドラへ進む", func(t *testing.T) {
		in, slept := newInjector(t, 0, running(chaos.KindLatency, route))

		_, handled := serve(in)

		if handled || *slept != 2*time.Second {
			t.Fatalf("handled = %v, slept = %v", handled, *slept)
		}
	})

	t.Run("http_error はハンドラへ進まず、本物の失敗と同じ本文で返す", func(t *testing.T) {
		in, _ := newInjector(t, 0, running(chaos.KindHTTPError, route))

		w, handled := serve(in)

		if !handled || w.Code != http.StatusServiceUnavailable {
			t.Fatalf("handled = %v, status = %d", handled, w.Code)
		}
		if body := w.Body.String(); strings.Contains(body, "chaos") || strings.Contains(body, "障害注入") {
			t.Errorf("画面に障害注入だと出ている: %s", body)
		}
	})

	t.Run("db_error・outbound_timeout ではリクエストを止めない", func(t *testing.T) {
		in, slept := newInjector(t, 0, running(chaos.KindDBError, route))

		_, handled := serve(in)

		if handled || *slept != 0 {
			t.Fatalf("handled = %v, slept = %v", handled, *slept)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransport(t *testing.T) {
	const route = "GET /api/blog"
	called := false
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	t.Run("当たれば相手に送らず、待ってからタイムアウトの誤りを返す", func(t *testing.T) {
		called = false
		in, slept := newInjector(t, 0, running(chaos.KindOutboundTimeout, route))
		ctx, _ := requestCtx(route)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.microcms.io/api/v1/blogs", nil)

		res, err := in.Transport(base).RoundTrip(req)

		var ne net.Error
		if res != nil || !errors.As(err, &ne) || !ne.Timeout() {
			t.Fatalf("res = %v, err = %v", res, err)
		}
		if called || *slept != 2*time.Second {
			t.Errorf("called = %v, slept = %v", called, *slept)
		}
	})

	t.Run("当たらなければそのまま送る", func(t *testing.T) {
		called = false
		in, _ := newInjector(t, 0.99, running(chaos.KindOutboundTimeout, route))
		ctx, _ := requestCtx(route)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.microcms.io/api/v1/blogs", nil)

		res, err := in.Transport(base).RoundTrip(req)

		if err != nil || res.StatusCode != http.StatusOK || !called {
			t.Fatalf("res = %v, err = %v, called = %v", res, err, called)
		}
	})
}

func TestHidesFaults(t *testing.T) {
	// 調べる練習で答えにならないよう、/metrics にも誤りの文言にも障害注入の印を出さない（JUK-178）。
	m := telemetry.NewMetrics(nil)
	in := New()
	in.now = func() time.Time { return t0 }
	in.active.Store(&[]chaos.Experiment{running(chaos.KindLatency, chaos.AllRoutes)})
	rec := httptest.NewRecorder()

	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	for _, out := range []string{rec.Body.String(), ErrInjected.Error(), outboundTimeout{}.Error()} {
		if strings.Contains(out, "chaos") || strings.Contains(out, "障害注入") {
			t.Errorf("障害注入の印が出ている: %.200s", out)
		}
	}
}
