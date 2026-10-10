package fault

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

var t0 = time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)

// newInjector は、時刻・くじ・待ち時間を固定した Injector に experiments を載せる。待った時間は slept に足す。
func newInjector(t *testing.T, roll float64, experiments ...Experiment) (*Injector, *time.Duration) {
	t.Helper()
	in := New(telemetry.NewMetrics(nil))
	in.now = func() time.Time { return t0 }
	in.roll = func() float64 { return roll }
	var slept time.Duration
	in.sleep = func(_ context.Context, d time.Duration) error { slept += d; return nil }
	in.active.Store(&experiments)
	return in, &slept
}

func running(kind Kind, route string) Experiment {
	return Experiment{ID: 1, Kind: kind, Route: route, Rate: 0.5, DelayMs: 2000, StatusCode: 503,
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
		e     Experiment
		ctx   string // リクエストの FaultRoute。空なら対象外のルート
		roll  float64
		want  bool
		reach bool // ctx にリクエストの情報があるか（無ければ、リクエストの外の処理）
	}{
		{name: "対象のルートでくじに当たれば起こす", e: running(KindDBError, route), ctx: route, roll: 0.49, want: true, reach: true},
		{name: "くじに外れれば起こさない", e: running(KindDBError, route), ctx: route, roll: 0.5, reach: true},
		{name: "ルートが違えば起こさない", e: running(KindDBError, route), ctx: "GET /api/goals", roll: 0, reach: true},
		{name: "* は /api/ のルート全部に起こす", e: running(KindDBError, AllRoutes), ctx: "GET /api/goals", roll: 0, want: true, reach: true},
		{name: "* でも /api/health には起こさない", e: running(KindDBError, AllRoutes), ctx: "GET /api/health", roll: 0, reach: true},
		{name: "/api/health は名指しすれば起こす", e: running(KindDBError, "GET /api/health"), ctx: "GET /api/health", roll: 0, want: true, reach: true},
		{name: "対象外のルート（管理画面・ログイン）には起こさない", e: running(KindDBError, AllRoutes), roll: 0, reach: true},
		{name: "リクエストの外の処理には起こさない", e: running(KindDBError, AllRoutes), roll: 0},
		{name: "種類が違えば起こさない", e: running(KindLatency, route), ctx: route, roll: 0, reach: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, _ := newInjector(t, tt.roll, tt.e)
			ctx, info := context.Background(), (*telemetry.RequestInfo)(nil)
			if tt.reach {
				ctx, info = requestCtx(tt.ctx)
			}

			_, got := in.pick(ctx, KindDBError)

			if got != tt.want {
				t.Fatalf("pick = %v, want %v", got, tt.want)
			}
			if info != nil && got != slices.Equal(info.Faults(), []string{"db_error"}) {
				t.Errorf("faults = %v", info.Faults())
			}
			if n := testutil.ToFloat64(in.injected.WithLabelValues("db_error")); n != map[bool]float64{true: 1}[got] {
				t.Errorf("chaos_faults_injected_total = %v", n)
			}
		})
	}

	t.Run("終わる時刻を過ぎたら、DB から読み直す前でも起こさない", func(t *testing.T) {
		e := running(KindDBError, AllRoutes)
		in, _ := newInjector(t, 0, e)
		in.now = func() time.Time { return e.EndsAt }
		ctx, _ := requestCtx(route)

		if _, got := in.pick(ctx, KindDBError); got {
			t.Fatal("終わった実験で障害を起こした")
		}
		if in.running(KindDBError) {
			t.Error("終わった実験が実行中のまま")
		}
	})

	t.Run("Injector が nil（無効）なら何もしない", func(t *testing.T) {
		var in *Injector
		ctx, _ := requestCtx(route)
		if _, got := in.pick(ctx, KindDBError); got {
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
		in, slept := newInjector(t, 0, running(KindLatency, route))

		_, handled := serve(in)

		if handled || *slept != 2*time.Second {
			t.Fatalf("handled = %v, slept = %v", handled, *slept)
		}
	})

	t.Run("http_error はハンドラへ進まず、本物の失敗と同じ本文で返す", func(t *testing.T) {
		in, _ := newInjector(t, 0, running(KindHTTPError, route))

		w, handled := serve(in)

		if !handled || w.Code != http.StatusServiceUnavailable {
			t.Fatalf("handled = %v, status = %d", handled, w.Code)
		}
		if body := w.Body.String(); strings.Contains(body, "chaos") || strings.Contains(body, "障害注入") {
			t.Errorf("画面に障害注入だと出ている: %s", body)
		}
	})

	t.Run("db_error・outbound_timeout ではリクエストを止めない", func(t *testing.T) {
		in, slept := newInjector(t, 0, running(KindDBError, route))

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
		in, slept := newInjector(t, 0, running(KindOutboundTimeout, route))
		ctx, info := requestCtx(route)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.microcms.io/api/v1/blogs", nil)

		res, err := in.Transport(base).RoundTrip(req)

		var ne net.Error
		if res != nil || !errors.As(err, &ne) || !ne.Timeout() {
			t.Fatalf("res = %v, err = %v", res, err)
		}
		if called || *slept != 2*time.Second {
			t.Errorf("called = %v, slept = %v", called, *slept)
		}
		if !slices.Equal(info.Faults(), []string{"outbound_timeout"}) {
			t.Errorf("faults = %v", info.Faults())
		}
	})

	t.Run("当たらなければそのまま送る", func(t *testing.T) {
		called = false
		in, _ := newInjector(t, 0.99, running(KindOutboundTimeout, route))
		ctx, _ := requestCtx(route)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.microcms.io/api/v1/blogs", nil)

		res, err := in.Transport(base).RoundTrip(req)

		if err != nil || res.StatusCode != http.StatusOK || !called {
			t.Fatalf("res = %v, err = %v, called = %v", res, err, called)
		}
	})
}

func TestSpecValidate(t *testing.T) {
	routes := []string{"GET /api/study-logs", "GET /api/health"}
	valid := map[Kind]Spec{
		KindLatency:         {Kind: KindLatency, Route: AllRoutes, Rate: 0.3, DelayMs: 2000, Duration: 10 * time.Minute},
		KindHTTPError:       {Kind: KindHTTPError, Route: "GET /api/study-logs", Rate: 1, StatusCode: 503, Duration: time.Minute},
		KindDBError:         {Kind: KindDBError, Route: AllRoutes, Rate: 0.1, Duration: time.Hour},
		KindOutboundTimeout: {Kind: KindOutboundTimeout, Route: AllRoutes, Rate: 0.5, DelayMs: MaxDelayMs, Duration: time.Minute},
	}
	for kind, s := range valid {
		if err := s.Validate(routes); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}

	invalid := []struct {
		name string
		edit func(*Spec)
	}{
		{"知らない種類", func(s *Spec) { s.Kind = "disk_full" }},
		{"対象外のルート", func(s *Spec) { s.Route = "GET /api/admin/chaos" }},
		{"割合が 0", func(s *Spec) { s.Rate = 0 }},
		{"割合が 1 を超える", func(s *Spec) { s.Rate = 1.01 }},
		{"終わる時刻が無い（長さ 0）", func(s *Spec) { s.Duration = 0 }},
		{"1時間を超える", func(s *Spec) { s.Duration = MaxDuration + time.Second }},
		{"待ち時間がリクエストの上限に届く", func(s *Spec) { s.DelayMs = MaxDelayMs + 1 }},
		{"遅延に statusCode", func(s *Spec) { s.StatusCode = 500 }},
	}
	for _, tt := range invalid {
		s := valid[KindLatency]
		tt.edit(&s)
		if err := s.Validate(routes); err == nil {
			t.Errorf("%s: 通ってしまった", tt.name)
		}
	}
	if err := (Spec{Kind: KindHTTPError, Route: AllRoutes, Rate: 1, StatusCode: 404, Duration: time.Minute}).Validate(routes); err == nil {
		t.Error("5xx でないステータスが通ってしまった")
	}
	if err := (Spec{Kind: KindDBError, Route: AllRoutes, Rate: 1, DelayMs: 10, Duration: time.Minute}).Validate(routes); err == nil {
		t.Error("db_error の delayMs が通ってしまった")
	}
}

func TestMetrics(t *testing.T) {
	// /metrics に、種類ごとの実行中の印と起こした数が、障害を起こす前から 0 で出る（アラートの increase() が数えられるように）。
	m := telemetry.NewMetrics(nil)
	in := New(m)
	in.now = func() time.Time { return t0 }
	in.active.Store(&[]Experiment{running(KindLatency, AllRoutes)})
	rec := httptest.NewRecorder()

	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`chaos_experiment_active{kind="latency"} 1`,
		`chaos_experiment_active{kind="db_error"} 0`,
		`chaos_faults_injected_total{kind="outbound_timeout"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics に %s が無い", want)
		}
	}
}
