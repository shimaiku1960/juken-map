package study

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/aiparse"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

// fakeParser は Claude の代わり。受け取った Input を残し、決まった結果を返す。
type fakeParser struct {
	result aiparse.Result
	err    error
	got    aiparse.Input
	calls  int
}

func (f *fakeParser) Parse(_ context.Context, in aiparse.Input) (aiparse.Result, error) {
	f.calls++
	f.got = in
	return f.result, f.err
}

var parseBooks = []aiparse.Textbook{
	{ID: 1, Name: "青チャート数学IA", Subject: "math"},
	{ID: 2, Name: "ターゲット1900", Subject: "english"},
}

// 2026-10-11 09:00 JST
var parseNow = time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)

func newTestParseHandler(p aiparse.Parser) *ParseHandler {
	h := NewParseHandler(nil, p)
	h.textbooks = func(context.Context, string) ([]aiparse.Textbook, error) { return parseBooks, nil }
	h.now = func() time.Time { return parseNow }
	return h
}

func postParse(h *ParseHandler, userID, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/study-logs/parse", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Serve(w, r, &httpx.Session{UserID: userID})
	return w
}

func ptr[T any](v T) *T { return &v }

func TestParseHandler(t *testing.T) {
	p := &fakeParser{result: aiparse.Result{Subject: ptr("math"), Minutes: ptr(int64(40)), TextbookID: ptr(int64(1))}}
	h := newTestParseHandler(p)

	w := postParse(h, "u1", `{"text":"  青チャート40分  "}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	httpxtest.AssertJSONEqual(t, w.Body.String(), `{"date":null,"subject":"math","minutes":40,"textbookId":1}`)
	if p.got.Text != "青チャート40分" || p.got.Today != "2026-10-11" || len(p.got.Textbooks) != 2 {
		t.Errorf("parser got %+v", p.got)
	}
}

func TestParseHandlerInput(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"空", `{"text":""}`, httpxtest.IssueJSON("記録したい内容を入力してください", "too_small", "text")},
		{"空白だけ", `{"text":"   "}`, httpxtest.IssueJSON("記録したい内容を入力してください", "too_small", "text")},
		{"201文字", `{"text":"` + strings.Repeat("あ", 201) + `"}`, httpxtest.IssueJSON("200文字以内で入力してください", "too_big", "text")},
		{"文字列でない", `{"text":1}`, httpxtest.IssueJSON("Invalid input: expected string, received number", "invalid_type", "text")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &fakeParser{}
			w := postParse(newTestParseHandler(p), "u1", tt.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s", w.Code, w.Body)
			}
			httpxtest.AssertJSONEqual(t, w.Body.String(), tt.want)
			if p.calls != 0 {
				t.Error("弾いた入力で Claude を呼んだ")
			}
		})
	}
	// 200文字ちょうどは通る
	if w := postParse(newTestParseHandler(&fakeParser{}), "u1", `{"text":"`+strings.Repeat("あ", 200)+`"}`); w.Code != http.StatusOK {
		t.Errorf("200文字: status = %d", w.Code)
	}
}

func TestParseHandlerUnavailable(t *testing.T) {
	w := postParse(newTestParseHandler(nil), "u1", `{"text":"数学30分"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", w.Code)
	}
	httpxtest.AssertJSONEqual(t, w.Body.String(), `{"error":"`+parseUnavailableMessage+`"}`)
}

func TestParseHandlerParserError(t *testing.T) {
	w := postParse(newTestParseHandler(&fakeParser{err: errors.New("boom")}), "u1", `{"text":"数学30分"}`)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", w.Code)
	}
	// 原因（boom）は画面に出さない
	httpxtest.AssertJSONEqual(t, w.Body.String(), `{"error":"`+parseFailedMessage+`"}`)
}

func TestParseHandlerQuota(t *testing.T) {
	p := &fakeParser{}
	h := newTestParseHandler(p)
	for i := range parseUserDaily {
		if w := postParse(h, "u1", `{"text":"数学30分"}`); w.Code != http.StatusOK {
			t.Fatalf("%d回目: status = %d", i+1, w.Code)
		}
	}
	w := postParse(h, "u1", `{"text":"数学30分"}`)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("上限の次: status = %d", w.Code)
	}
	httpxtest.AssertJSONEqual(t, w.Body.String(), `{"error":"`+parseUserLimitMessage+`"}`)
	if p.calls != parseUserDaily {
		t.Errorf("calls = %d", p.calls)
	}
	// ほかの人は使える
	if w := postParse(h, "u2", `{"text":"数学30分"}`); w.Code != http.StatusOK {
		t.Errorf("u2: status = %d", w.Code)
	}
	// 日本時間の0時（UTC 15:00）を過ぎたら戻る
	h.now = func() time.Time { return time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC) }
	if w := postParse(h, "u1", `{"text":"数学30分"}`); w.Code != http.StatusOK {
		t.Errorf("翌日: status = %d", w.Code)
	}
}

func TestDailyQuotaTotal(t *testing.T) {
	q := &dailyQuota{userMax: 2, totalMax: 3}
	for _, u := range []string{"a", "a", "b"} {
		if got := q.take("2026-10-11", u); got != quotaOK {
			t.Fatalf("%s: %v", u, got)
		}
	}
	if got := q.take("2026-10-11", "a"); got != quotaUserFull {
		t.Errorf("a: %v", got)
	}
	if got := q.take("2026-10-11", "c"); got != quotaTotalFull {
		t.Errorf("c: %v", got)
	}
	if got := q.take("2026-10-12", "c"); got != quotaOK {
		t.Errorf("翌日: %v", got)
	}
}

func TestCheckParseResult(t *testing.T) {
	const today = "2026-10-11"
	tests := []struct {
		name string
		in   aiparse.Result
		want string
	}{
		{"全部正しい", aiparse.Result{Date: ptr("2026-10-10"), Subject: ptr("english"), Minutes: ptr(int64(30)), TextbookID: ptr(int64(2))},
			`{"date":"2026-10-10","subject":"english","minutes":30,"textbookId":2}`},
		{"全部 null", aiparse.Result{}, `{"date":null,"subject":null,"minutes":null,"textbookId":null}`},
		{"今日は通る", aiparse.Result{Date: ptr(today)}, `{"date":"2026-10-11","subject":null,"minutes":null,"textbookId":null}`},
		{"未来の日付", aiparse.Result{Date: ptr("2026-10-12"), Minutes: ptr(int64(30))}, `{"date":null,"subject":null,"minutes":30,"textbookId":null}`},
		{"暦に無い日付", aiparse.Result{Date: ptr("2026-02-30")}, `{"date":null,"subject":null,"minutes":null,"textbookId":null}`},
		{"形の違う日付", aiparse.Result{Date: ptr("10/10")}, `{"date":null,"subject":null,"minutes":null,"textbookId":null}`},
		{"知らない科目", aiparse.Result{Subject: ptr("physics")}, `{"date":null,"subject":null,"minutes":null,"textbookId":null}`},
		{"0分", aiparse.Result{Minutes: ptr(int64(0))}, `{"date":null,"subject":null,"minutes":null,"textbookId":null}`},
		{"1440分は通る", aiparse.Result{Minutes: ptr(int64(1440))}, `{"date":null,"subject":null,"minutes":1440,"textbookId":null}`},
		{"1441分", aiparse.Result{Minutes: ptr(int64(1441))}, `{"date":null,"subject":null,"minutes":null,"textbookId":null}`},
		// 他人の参考書の ID を出させても、本人の一覧に無ければ返さない
		{"本人の参考書に無い ID", aiparse.Result{Subject: ptr("math"), TextbookID: ptr(int64(999))}, `{"date":null,"subject":"math","minutes":null,"textbookId":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkParseResult(tt.in, parseBooks, today)
			w := httptest.NewRecorder()
			httpx.WriteJSON(w, http.StatusOK, got)
			httpxtest.AssertJSONEqual(t, w.Body.String(), tt.want)
		})
	}
}
