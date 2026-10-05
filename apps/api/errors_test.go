package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewErrorBody(t *testing.T) {
	// 文言の選び方は Node の errorBody（error-handling.ts）と同じ。
	tests := []struct {
		status int
		code   ServerErrorCode
		want   string
	}{
		{500, codeInternal, serverMessage},
		{503, codeOverloaded, overloadedMessage},
		{404, codeNotFound, fallbackClientMessage},
	}
	for _, tt := range tests {
		got := newErrorBody(tt.status, tt.code, "req-1")
		if got != (ServerError{Error: tt.want, Code: tt.code, ReqID: "req-1"}) {
			t.Errorf("newErrorBody(%d, %q) = %+v", tt.status, tt.code, got)
		}
	}
}

func TestPathID(t *testing.T) {
	// Node の idParamsSchema と同じ境目（routes/params.test.ts）。
	tests := []struct {
		raw    string
		want   int64
		wantOK bool
	}{
		{"1", 1, true},
		{"42", 42, true},
		{"999999999999999", 999999999999999, true}, // 15桁
		{"1000000000000000", 0, false},             // 16桁
		{"0", 0, false},
		{"01", 0, false},
		{"-1", 0, false},
		{"1.5", 0, false},
		{"1e3", 0, false},
		{"abc", 0, false},
		{" 1", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.SetPathValue("id", tt.raw)
			res := httptest.NewRecorder()

			got, ok := pathID(res, req, "id")
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("pathID(%q) = (%d, %v), want (%d, %v)", tt.raw, got, ok, tt.want, tt.wantOK)
			}
			if !ok {
				if res.Code != http.StatusBadRequest {
					t.Errorf("status = %d, want 400", res.Code)
				}
				assertJSONEqual(t, res.Body.String(), `{"error":"ID が正しくありません"}`)
			}
		})
	}
}
