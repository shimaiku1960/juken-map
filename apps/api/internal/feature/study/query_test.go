package study

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

func TestStudyListQueryErrors(t *testing.T) {
	// DB に届く前に返る場面だけを見る（store は nil のまま）。DB を使う場面は E2E（nginx を通して Go に届く、JUK-96）が通す。
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	h := &ReadHandlers{}
	rt.User("GET /api/study-logs", h.ListLogs)
	rt.User("GET /api/study-logs/daily", h.ListDaily)
	rt.User("GET /api/study-plans", h.ListPlans)

	// 本文は入力チェックのほかの 400 と同じ「文言・コード・項目」
	formatError := func(key string) string {
		return `{"error":"日付は YYYY-MM-DD で指定してください","code":"invalid_format","field":"` + key + `"}`
	}
	arrayError := func(key string) string {
		return `{"error":"Invalid input: expected string, received array","code":"invalid_type","field":"` + key + `"}`
	}

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/api/study-logs?from=abc", 400, formatError("from")},
		{"/api/study-logs?from=abc&to=x", 400, formatError("from")},
		{"/api/study-logs?from", 400, formatError("from")},
		{"/api/study-logs?from=2026-09-28;to=x", 400, formatError("from")},
		{"/api/study-logs/daily?to=bad", 400, formatError("to")},
		{"/api/study-plans?to=2026-09-01&to=2026-09-02", 400, arrayError("to")},
		// 形は正しいが暦に無い日付は、200 の空の一覧
		{"/api/study-logs?from=2026-13-45", 200, `[]`},
		{"/api/study-logs/daily?from=2026-01-01&to=2026-00-10", 200, `[]`},
		{"/api/study-plans?from=2026-01-32", 200, `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			req.AddCookie(&http.Cookie{Name: "test", Value: "alice"})
			res := httptest.NewRecorder()
			rt.ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d（本文 %s）", res.Code, tt.wantStatus, res.Body)
			}
			httpxtest.AssertJSONEqual(t, res.Body.String(), tt.wantBody)
		})
	}
}
