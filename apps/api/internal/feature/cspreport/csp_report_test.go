package cspreport

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// postCSP は CSP の報告を送り、ステータスと "csp violation" のログの csp の中身を返す。
func postCSP(t *testing.T, body, contentType string) (int, []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(telemetry.NewLogger(&buf, slog.LevelDebug))
	t.Cleanup(func() { slog.SetDefault(prev) })
	req := httptest.NewRequest("POST", "/api/csp-report", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	res := httptest.NewRecorder()
	Handle(res, req)

	var reports []map[string]any
	sc := bufio.NewScanner(&buf)
	for sc.Scan() {
		line := httpxtest.DecodeJSON(t, sc.Text())
		if line["msg"] == "csp violation" {
			reports = append(reports, line["csp"].(map[string]any))
		}
	}
	return res.Code, reports
}

// 以下の3つは Node の csp-report.test.ts と同じ入力と期待値。

func TestCSPReportURI(t *testing.T) {
	// report-uri の形（Safari・Firefox）を受けてログに残す
	status, reports := postCSP(t, `{"csp-report": {
		"document-uri": "https://juken-map.com/reset-password?token=secret",
		"effective-directive": "style-src-elem",
		"blocked-uri": "inline",
		"line-number": 3}}`, "application/csp-report")

	if status != 204 || len(reports) != 1 {
		t.Fatalf("status = %d, reports = %v", status, reports)
	}
	want := map[string]any{
		"documentUrl": "https://juken-map.com/reset-password",
		"directive":   "style-src-elem",
		"blockedUrl":  "inline",
		"lineNumber":  float64(3),
	}
	if !reflect.DeepEqual(reports[0], want) {
		t.Errorf("csp = %v, want %v", reports[0], want)
	}
}

func TestCSPReportTo(t *testing.T) {
	// report-to の形（Chrome）を受け、トークンを伏せてログに残す。csp-violation 以外は捨てる
	status, reports := postCSP(t, `[
		{"type": "csp-violation", "body": {
			"documentURL": "https://juken-map.com/api/auth/reset-password/tok123?callbackURL=x",
			"effectiveDirective": "img-src",
			"blockedURL": "https://evil.example/pixel.gif?leak=1",
			"disposition": "report"}},
		{"type": "deprecation", "body": {"id": "x"}}]`, "application/reports+json")

	if status != 204 || len(reports) != 1 {
		t.Fatalf("status = %d, reports = %v", status, reports)
	}
	want := map[string]any{
		"documentUrl": "https://juken-map.com/api/auth/reset-password/:token",
		"directive":   "img-src",
		"blockedUrl":  "https://evil.example/pixel.gif",
		"disposition": "report",
	}
	if !reflect.DeepEqual(reports[0], want) {
		t.Errorf("csp = %v, want %v", reports[0], want)
	}
}

func TestCSPReportBroken(t *testing.T) {
	// 壊れた本文でも 204 を返し、何も残さない
	status, reports := postCSP(t, "{not json", "application/csp-report")
	if status != 204 || len(reports) != 0 {
		t.Fatalf("status = %d, reports = %v", status, reports)
	}
}

func TestParseCSPReportsLimit(t *testing.T) {
	// 一度に読む報告は20件まで
	many := make([]any, 50)
	for i := range many {
		many[i] = map[string]any{"type": "csp-violation", "body": map[string]any{"effectiveDirective": "img-src"}}
	}
	if got := len(parseCSPReports(many)); got != 20 {
		t.Errorf("len = %d, want 20", got)
	}
}

func TestSafeURL(t *testing.T) {
	for _, tt := range []struct {
		in   any
		want string
	}{
		{"https://Juken-Map.com:443/path?q=1", "https://juken-map.com/path"},
		{"https://juken-map.com", "https://juken-map.com/"},
		{"http://localhost:5173/x#frag", "http://localhost:5173/x"},
		{"inline", "inline"},
		{"eval", "eval"},
		{strings.Repeat("あ", 250), strings.Repeat("あ", 200)},
		{float64(1), ""},
		{nil, ""},
	} {
		t.Run(fmt.Sprint(tt.in), func(t *testing.T) {
			if got := safeURL(tt.in); got != tt.want {
				t.Errorf("safeURL(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
