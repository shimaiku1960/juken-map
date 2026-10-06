package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// recordSpans は送らずに手元へ溜めるトレースの設定を作る。終わったスパンを sr.Ended() で読める。
func recordSpans() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	sr := tracetest.NewSpanRecorder()
	return sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)), sr
}

// spanText はスパンの名前と属性の値をつないだ文字列。秘密が紛れ込んでいないかを見るのに使う。
func spanText(s sdktrace.ReadOnlySpan) string {
	parts := []string{s.Name()}
	for _, kv := range s.Attributes() {
		parts = append(parts, string(kv.Key)+"="+kv.Value.String())
	}
	return strings.Join(parts, " ")
}

func spanAttr(s sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.String()
		}
	}
	return ""
}

func TestRequestSpanUsesRouteTemplate(t *testing.T) {
	buf := captureLogs(t)
	tp, sr := recordSpans()
	h := newServerHandler(newTestRouter(), newMetrics(), serverOptions{maxInFlight: 10, tracer: tp.Tracer("test")})

	res := serve(h, "DELETE", "/api/mine/log-123?token=secret-in-query", "alice")

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("スパンの数 = %d, want 1", len(spans))
	}
	s := spans[0]
	// ダッシュボードの絞り込み（trace:rootName）と同じ、Node の Fastify の形の名前にする。
	if s.Name() != "DELETE /api/mine/:id" || s.SpanKind() != trace.SpanKindServer {
		t.Errorf("名前 = %q, 種類 = %v", s.Name(), s.SpanKind())
	}
	if spanAttr(s, "http.response.status_code") != "200" || spanAttr(s, "reqId") != res.Header().Get("X-Request-Id") {
		t.Errorf("属性 = %s", spanText(s))
	}
	// パスの値・クエリのトークン・Cookie はスパンに入れない。
	for _, secret := range []string{"log-123", "secret-in-query", "alice"} {
		if strings.Contains(spanText(s), secret) {
			t.Errorf("スパンに %q が残っている: %s", secret, spanText(s))
		}
	}
	// ログの行の trace_id から、このトレースを開ける（Grafana の Loki → Tempo のリンク）。
	line := findLog(logLines(t, buf), "request completed")
	if line["trace_id"] != s.SpanContext().TraceID().String() || line["span_id"] != s.SpanContext().SpanID().String() {
		t.Errorf("trace_id = %v, span_id = %v, want %s / %s",
			line["trace_id"], line["span_id"], s.SpanContext().TraceID(), s.SpanContext().SpanID())
	}
}

func TestRequestSpanStatus(t *testing.T) {
	captureLogs(t)
	tp, sr := recordSpans()
	rt := newRouter(fakeSessions(nil))
	rt.public("GET /api/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	rt.public("GET /api/missing", func(w http.ResponseWriter, r *http.Request) {
		writeErrorBody(w, r, http.StatusNotFound, codeNotFound)
	})
	h := newServerHandler(rt, newMetrics(), serverOptions{maxInFlight: 10, tracer: tp.Tracer("test")})

	serve(h, "GET", "/api/boom", "")
	serve(h, "GET", "/api/missing", "")
	serve(h, "GET", "/api/nowhere", "")

	want := []struct {
		name   string
		status codes.Code
	}{
		{"GET /api/boom", codes.Error}, // 5xx はこちらの誤りなのでエラーにする
		{"GET /api/missing", codes.Unset},
		{"GET (unmatched)", codes.Unset}, // ルートに当たらなくても、パスの値は名前に入れない
	}
	spans := sr.Ended()
	if len(spans) != len(want) {
		t.Fatalf("スパンの数 = %d, want %d", len(spans), len(want))
	}
	for i, w := range want {
		if spans[i].Name() != w.name || spans[i].Status().Code != w.status {
			t.Errorf("%d: 名前 = %q, 状態 = %v, want %q / %v", i, spans[i].Name(), spans[i].Status().Code, w.name, w.status)
		}
	}
}

func TestNoTracerStillServes(t *testing.T) {
	buf := captureLogs(t)
	h, _ := newTestServer(newMetrics(), 10)

	if res := serve(h, "GET", "/api/public", ""); res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	// トレースを取らないときは、ログに trace_id を書かない（空の ID で Tempo へのリンクを作らせない）。
	if line := findLog(logLines(t, buf), "request completed"); line["trace_id"] != nil {
		t.Errorf("trace_id = %v, want 無し", line["trace_id"])
	}
}

func TestOutboundClientSpan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 外部サービスへ自分のトレースの ID は渡さない。
		if r.Header.Get("traceparent") != "" {
			t.Errorf("traceparent を送っている: %s", r.Header.Get("traceparent"))
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	tp, sr := recordSpans()
	client := telemetry.NewOutboundClient(tp)

	// リクエストの外（親のスパンが無い）では、スパンを作らない。
	get(t, client, context.Background(), srv.URL)
	if n := len(sr.Ended()); n != 0 {
		t.Fatalf("親が無いのにスパンが %d 個できた", n)
	}

	ctx, parent := tp.Tracer("test").Start(context.Background(), "GET /api/x")
	get(t, client, ctx, srv.URL+"/v2/bot/message/push?token=secret-in-query")
	parent.End()

	spans := sr.Ended()
	if len(spans) != 2 {
		t.Fatalf("スパンの数 = %d, want 2", len(spans))
	}
	s := spans[0]
	host := strings.TrimPrefix(srv.URL, "http://")
	if s.Name() != "GET "+host || s.SpanKind() != trace.SpanKindClient || s.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Errorf("名前 = %q, 種類 = %v, 親 = %v", s.Name(), s.SpanKind(), s.Parent().SpanID())
	}
	if spanAttr(s, "http.response.status_code") != "429" || s.Status().Code != codes.Error {
		t.Errorf("属性 = %s, 状態 = %v", spanText(s), s.Status().Code)
	}
	// パスと ? 以降にはトークンが載ることがあるので、ホスト名だけを残す。
	for _, secret := range []string{"/v2/bot", "secret-in-query"} {
		if strings.Contains(spanText(s), secret) {
			t.Errorf("スパンに %q が残っている: %s", secret, spanText(s))
		}
	}
}

func get(t *testing.T, client *http.Client, ctx context.Context, url string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
}

func TestLogOutput(t *testing.T) {
	var stdout bytes.Buffer
	if w, err := telemetry.LogOutput(&stdout, ""); err != nil || w != &stdout {
		t.Fatalf("LOG_FILE が空なら標準出力だけにする: %v, %v", w, err)
	}

	// 無いディレクトリは作る（手元の logs/ は gitignore 済みで、clone した直後は無い）。
	path := filepath.Join(t.TempDir(), "logs", "api.log")
	for _, msg := range []string{"first", "second"} {
		w, err := telemetry.LogOutput(&stdout, path)
		if err != nil {
			t.Fatal(err)
		}
		telemetry.NewLogger(w, 0).Info(msg)
	}

	got, _ := os.ReadFile(path)
	// 起動し直しても前のログを消さず、同じ行を標準出力とファイルの両方に書く。
	if !strings.Contains(string(got), `"msg":"first"`) || !strings.Contains(string(got), `"msg":"second"`) {
		t.Errorf("ファイル = %s", got)
	}
	if !strings.Contains(stdout.String(), `"msg":"first"`) {
		t.Errorf("標準出力 = %s", stdout.String())
	}
}

func TestWebSpansAreNotSent(t *testing.T) {
	buf := captureLogs(t)
	sr := tracetest.NewSpanRecorder()
	// 本番と同じく、送る手前に telemetry.SkipWebSpans を挟む。
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.SkipWebSpans{SpanProcessor: sr}))
	h := newServerHandler(newSPATestRouter(t, pageScripts{}), newMetrics(), serverOptions{maxInFlight: 10, tracer: tp.Tracer("test")})

	serve(h, "GET", "/", "")
	serve(h, "GET", "/api/health", "")

	spans := sr.Ended()
	if len(spans) != 1 || spans[0].Name() != "GET /api/health" {
		names := []string{}
		for _, s := range spans {
			names = append(names, s.Name())
		}
		t.Fatalf("送ったスパン = %v, want [GET /api/health] だけ", names)
	}
	// 送らないトレースの ID はログに書かない。送るほうには書く。
	for _, line := range logLines(t, buf) {
		if line["msg"] != "request completed" {
			continue
		}
		url := line["req"].(map[string]any)["url"]
		if hasID := line["trace_id"] != nil; hasID != (url == "/api/health") {
			t.Errorf("%v の trace_id = %v", url, line["trace_id"])
		}
	}
}
