package main

import (
	"bufio"
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// captureLogs はテストの間だけ、既定のロガーの書き出し先を buf にする。
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(telemetry.NewLogger(&buf, slog.LevelDebug))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// logLines は書き出されたログを1行ずつ JSON として読む。
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	for sc.Scan() {
		lines = append(lines, decodeJSON(t, sc.Text()))
	}
	return lines
}

func findLog(lines []map[string]any, msg string) map[string]any {
	for _, l := range lines {
		if l["msg"] == msg {
			return l
		}
	}
	return nil
}

// serve は本番と同じミドルウェアを重ねた上で、1件だけ処理する。
func serve(h http.Handler, method, path, cookie string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "test", Value: cookie})
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func newTestServer(m *telemetry.Metrics, maxInFlight int) (http.Handler, *router) {
	rt := newTestRouter()
	return newServerHandler(rt, m, serverOptions{maxInFlight: maxInFlight}), rt
}

func TestAccessLogIsPinoShaped(t *testing.T) {
	buf := captureLogs(t)
	h, _ := newTestServer(newMetrics(), 10)

	res := serve(h, "GET", "/api/mine?token=secret-in-query", "alice")

	line := findLog(logLines(t, buf), "request completed")
	if line == nil {
		t.Fatalf("request completed の行が無い: %s", buf)
	}
	// Alloy と Grafana が読む項目（observability/alloy/production.alloy・grafana/dashboards/api.json）。
	if line["level"] != float64(30) {
		t.Errorf("level = %v, want 30", line["level"])
	}
	if _, ok := line["time"].(float64); !ok {
		t.Errorf("time が数字（UNIX ミリ秒）でない: %v", line["time"])
	}
	for _, key := range []string{"pid", "hostname", "responseTime"} {
		if _, ok := line[key]; !ok {
			t.Errorf("%s が無い", key)
		}
	}
	if line["reqId"] != res.Header().Get("X-Request-Id") || line["reqId"] == "" {
		t.Errorf("reqId = %v, X-Request-Id = %q", line["reqId"], res.Header().Get("X-Request-Id"))
	}
	req, _ := line["req"].(map[string]any)
	if req["method"] != "GET" || req["url"] != "/api/mine" {
		t.Errorf("req = %v（URL はパスだけにする）", req)
	}
	resLog, _ := line["res"].(map[string]any)
	if resLog["statusCode"] != float64(200) {
		t.Errorf("res = %v", resLog)
	}
	// クエリに載ったトークンと Cookie の値はどの行にも残さない。
	if strings.Contains(buf.String(), "secret-in-query") || strings.Contains(buf.String(), "alice") {
		t.Errorf("ログに秘密が残っている: %s", buf)
	}
}

func TestAccessLogMarksSimulation(t *testing.T) {
	buf := captureLogs(t)
	h, _ := newTestServer(newMetrics(), 10)

	serve(h, "GET", "/api/public", "", "X-Sim-Run", "run-1")

	if line := findLog(logLines(t, buf), "request completed"); line["sim"] != true {
		t.Errorf("sim = %v, want true", line["sim"])
	}
}

func TestRequestID(t *testing.T) {
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if id := newRequestID(false); !uuid.MatchString(id) {
		t.Errorf("newRequestID(false) = %q", id)
	}
	if id := newRequestID(true); len(id) != 8 {
		t.Errorf("newRequestID(true) = %q, want 8文字", id)
	}
	if a, b := newRequestID(false), newRequestID(false); a == b {
		t.Error("同じ reqId が2回出た")
	}
}

func TestMetricsUseRouteTemplate(t *testing.T) {
	captureLogs(t)
	m := newMetrics()
	h, _ := newTestServer(m, 10)

	serve(h, "DELETE", "/api/mine/5", "alice")
	serve(h, "DELETE", "/api/mine/6", "alice")
	serve(h, "GET", "/api/nothing", "")
	serve(h, "GET", "/api/mine", "") // ハンドラの手前で断った 401 も数える

	tests := []struct {
		method, route, status string
		want                  float64
	}{
		{"DELETE", "/api/mine/:id", "200", 2},
		{"GET", "(unmatched)", "404", 1},
		{"GET", "/api/mine", "401", 1},
	}
	for _, tt := range tests {
		got := testutil.ToFloat64(m.Requests.WithLabelValues(tt.method, tt.route, tt.status))
		if got != tt.want {
			t.Errorf("http_requests_total{%s %s %s} = %v, want %v", tt.method, tt.route, tt.status, got, tt.want)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	captureLogs(t)
	h, _ := newTestServer(newMetrics(), 10)

	// 断った応答にも付くこと。
	res := serve(h, "GET", "/api/nothing", "")
	for name, want := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Strict-Transport-Security": "max-age=31536000",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := res.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestNotFoundBody(t *testing.T) {
	captureLogs(t)
	h, _ := newTestServer(newMetrics(), 10)

	res := serve(h, "GET", "/api/nothing", "")
	body := decodeJSON(t, res.Body.String())
	// Node の spa.ts と同じ形。reqId は応答ヘッダーと同じ値。
	if body["code"] != string(codeNotFound) || body["error"] != fallbackClientMessage || body["reqId"] != res.Header().Get("X-Request-Id") {
		t.Errorf("本文 = %v", body)
	}
	if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestRecoverPanic(t *testing.T) {
	buf := captureLogs(t)
	rt := newRouter(fakeSessions(nil))
	rt.public("GET /api/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("SELECT * FROM secret_table")
	})
	h := newServerHandler(rt, newMetrics(), serverOptions{maxInFlight: 10})

	res := serve(h, "GET", "/api/boom", "")

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.Code)
	}
	// 利用者には固定の文言だけを返し、原因はログにだけ残す。
	if strings.Contains(res.Body.String(), "secret_table") {
		t.Errorf("原因が応答に出ている: %s", res.Body)
	}
	body := decodeJSON(t, res.Body.String())
	if body["code"] != string(codeInternal) || body["error"] != serverMessage {
		t.Errorf("本文 = %v", body)
	}
	lines := logLines(t, buf)
	failed := findLog(lines, "request failed")
	if failed == nil || failed["level"] != float64(50) || !strings.Contains(failed["err"].(string), "secret_table") {
		t.Errorf("request failed の行 = %v", failed)
	}
	if done := findLog(lines, "request completed"); done["res"].(map[string]any)["statusCode"] != float64(500) {
		t.Errorf("request completed の行 = %v", done)
	}
}

func TestLimitInFlight(t *testing.T) {
	buf := captureLogs(t)
	entered, release := make(chan struct{}), make(chan struct{})
	rt := newRouter(fakeSessions(nil))
	rt.public("GET /api/slow", func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	rt.public("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	rt.public("GET /assets/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := newServerHandler(rt, newMetrics(), serverOptions{maxInFlight: 1})

	// 1件目を処理中のまま止めておく。
	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- serve(h, "GET", "/api/slow", "") }()
	<-entered

	// 上限に達しているので、2件目は待たずに 503。
	res := serve(h, "GET", "/api/slow", "")
	if res.Code != http.StatusServiceUnavailable || res.Header().Get("Retry-After") != "1" {
		t.Fatalf("status = %d, Retry-After = %q", res.Code, res.Header().Get("Retry-After"))
	}
	body := decodeJSON(t, res.Body.String())
	if body["code"] != string(codeOverloaded) || body["error"] != overloadedMessage {
		t.Errorf("本文 = %v", body)
	}
	if shed := findLog(logLines(t, buf), "request shed: overloaded"); shed == nil || shed["level"] != float64(40) {
		t.Errorf("断ったログ = %v", shed)
	}

	// 死活監視は混んでいても数えない。
	if res := serve(h, "GET", "/api/health", ""); res.Code != http.StatusOK {
		t.Errorf("/api/health = %d, want 200", res.Code)
	}
	// 画面と静的ファイル（/api/ の外）も数えない（画面を開くと何十本も同時に読むため）。
	if res := serve(h, "GET", "/assets/app.js", ""); res.Code != http.StatusOK {
		t.Errorf("/assets/app.js = %d, want 200", res.Code)
	}

	// 1件目が終われば、枠が空いて次を受け付ける。
	close(release)
	if first := <-done; first.Code != http.StatusOK {
		t.Fatalf("1件目 = %d", first.Code)
	}
	go func() { <-entered }()
	if res := serve(h, "GET", "/api/slow", ""); res.Code != http.StatusOK {
		t.Errorf("空いた後 = %d, want 200", res.Code)
	}
}

func TestRequestDeadline(t *testing.T) {
	captureLogs(t)
	var deadline time.Time
	var ok bool
	rt := newRouter(fakeSessions(nil))
	rt.public("GET /api/check", func(w http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	})
	h := newServerHandler(rt, newMetrics(), serverOptions{maxInFlight: 10})

	serve(h, "GET", "/api/check", "")

	if !ok {
		t.Fatal("ハンドラの ctx に期限が付いていない")
	}
	if left := time.Until(deadline); left <= 0 || left > requestTimeout {
		t.Errorf("残り時間 = %v, want 0〜%v", left, requestTimeout)
	}
}

func TestParseLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"":      slog.LevelInfo,
		"info":  slog.LevelInfo,
		"debug": slog.LevelDebug,
		"trace": slog.LevelDebug,
		"WARN":  slog.LevelWarn,
		"error": slog.LevelError,
		"what":  slog.LevelInfo,
	}
	for name, want := range tests {
		if got := telemetry.ParseLevel(name); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", name, got, want)
		}
	}
}
