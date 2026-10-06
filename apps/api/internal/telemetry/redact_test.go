package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// dupEntry は MySQL の重複エラー（1062）の文。値がそのまま入る。
const dupEntry = "Error 1062 (23000): Duplicate entry 'taro.yamada+1@example.co.jp' for key 'user.user_email_key'"

func TestRedactEmails(t *testing.T) {
	cases := []struct{ in, want string }{
		{dupEntry, "Error 1062 (23000): Duplicate entry '[REDACTED]' for key 'user.user_email_key'"},
		{"/login?email=taro%40example.com&x=1", "/login?email=[REDACTED]&x=1"},
		{"GET /api/users/:id", "GET /api/users/:id"},
		{"@grafana/faro-web-sdk", "@grafana/faro-web-sdk"},
	}
	for _, c := range cases {
		if got := redactEmails(c.in); got != c.want {
			t.Errorf("redactEmails(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLoggerRedactsEmails(t *testing.T) {
	var buf bytes.Buffer
	NewLogger(&buf, slog.LevelInfo).Error("sign-up failed for taro@example.com",
		"err", errors.New(dupEntry), "detail", "to=hanako@example.com",
		slog.Group("req", "note", "jiro@example.com"))

	out := buf.String()
	if strings.Contains(out, "@example.") {
		t.Fatalf("メールアドレスがログに残った: %s", out)
	}
	if strings.Count(out, "[REDACTED]") != 4 || !strings.Contains(out, `"level":50`) {
		t.Fatalf("伏せ方かログの形が違う: %s", out)
	}
}

func TestExporterRedactsEmails(t *testing.T) {
	mem := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(redactingExporter{mem}))
	_, span := tp.Tracer("test").Start(context.Background(), "INSERT")
	span.SetAttributes(attribute.String("note", "taro@example.com"), attribute.Int("n", 1))
	// otelsql は SQL が失敗するとこう記録する（RecordError で exception.message に誤りの文が入る）。
	span.RecordError(errors.New(dupEntry))
	span.SetStatus(codes.Error, dupEntry)
	span.End()

	spans := mem.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	s := spans[0]
	texts := []string{s.Status.Description}
	for _, kv := range s.Attributes {
		texts = append(texts, kv.Value.String())
	}
	for _, ev := range s.Events {
		for _, kv := range ev.Attributes {
			texts = append(texts, kv.Value.String())
		}
	}
	all := strings.Join(texts, " ")
	if strings.Contains(all, "@example.") || !strings.Contains(all, "Duplicate entry '[REDACTED]'") {
		t.Fatalf("メールアドレスがトレースに残った: %s", all)
	}
	if s.Status.Code != codes.Error || len(s.Events) != 1 {
		t.Fatalf("状態か誤りの記録が消えた: %+v", s)
	}
}
