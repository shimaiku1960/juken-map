package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestParseQuery(t *testing.T) {
	// 期待値は、同じクエリを Node（Fastify の既定の fast-querystring）に送って確かめたもの。
	tests := []struct {
		raw  string
		want map[string][]string
	}{
		{"", map[string][]string{}},
		{"from=2026-09-01&to=2026-09-30", map[string][]string{"from": {"2026-09-01"}, "to": {"2026-09-30"}}},
		{"from=a&from=b", map[string][]string{"from": {"a", "b"}}},
		// ; は区切りではない（Go の url.ParseQuery はこの組を捨てる）
		{"from=2026-09-28;to=x", map[string][]string{"from": {"2026-09-28;to=x"}}},
		// = の無い組は空文字
		{"from", map[string][]string{"from": {""}}},
		// 壊れた % は元の文字のまま（Go の url.ParseQuery はこの組を捨てる）
		{"from=%ZZ", map[string][]string{"from": {"%ZZ"}}},
		// キーも値もデコードする。+ は空白
		{"fr%6Fm=2026-09-2%38&to=a+b", map[string][]string{"from": {"2026-09-28"}, "to": {"a b"}}},
		{"&&from=1&", map[string][]string{"from": {"1"}}},
	}
	for _, tt := range tests {
		if got := parseQuery(tt.raw); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseQuery(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestParseYMD(t *testing.T) {
	// 期待値は Node で new Date("YYYY-MM-DD").toISOString() を確かめたもの。"" は Invalid Date。
	tests := map[string]string{
		"2026-09-29": "2026-09-29",
		"2024-02-29": "2024-02-29",
		"2026-02-29": "2026-03-01",
		"2026-02-30": "2026-03-02",
		"2026-04-31": "2026-05-01",
		"0000-01-01": "0000-01-01",
		"9999-12-31": "9999-12-31",
		"2026-13-45": "",
		"2026-00-10": "",
		"2026-01-00": "",
		"2026-01-32": "",
	}
	for in, want := range tests {
		got, ok := parseYMD(in)
		switch {
		case want == "" && ok:
			t.Errorf("parseYMD(%q) = %s, want Invalid", in, ymd(got))
		case want != "" && !ok:
			t.Errorf("parseYMD(%q) = Invalid, want %s", in, want)
		case want != "" && ymd(got) != want:
			t.Errorf("parseYMD(%q) = %s, want %s", in, ymd(got), want)
		}
	}
}

func TestDateRangeWhere(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	where, args := dateRange{from: from, to: &to}.where("l", "u1")
	if where != "l.userId = ? AND l.date >= ? AND l.date < ?" {
		t.Errorf("where = %q", where)
	}
	// to の当日を含めるため、上限は翌日の 00:00
	want := []any{"u1", from, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}

	where, args = dateRange{from: from}.where("p", "u1")
	if where != "p.userId = ? AND p.date >= ?" || len(args) != 2 {
		t.Errorf("上限なし: where = %q, args = %v", where, args)
	}
}

func TestStudyListQueryErrors(t *testing.T) {
	// DB に届く前に返る場面だけを見る（store は nil のまま）。DB を使う場面は E2E（nginx を通して Go に届く、JUK-96）が通す。
	rt := newRouter(fakeSessions(testSessions))
	h := &studyHandlers{}
	rt.user("GET /api/study-logs", h.listLogs)
	rt.user("GET /api/study-logs/daily", h.listDaily)
	rt.user("GET /api/study-plans", h.listPlans)

	// 本文は Node（routes/validation-error.ts）が返すものと同じ「文言・コード・項目」
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
		// 形は正しいが暦に無い日付は、Node と同じく 200 の空の一覧
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
			assertJSONEqual(t, res.Body.String(), tt.wantBody)
		})
	}
}
