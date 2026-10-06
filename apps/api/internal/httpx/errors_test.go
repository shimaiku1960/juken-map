package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
)

func TestNewErrorBody(t *testing.T) {
	// 文言の選び方は Node の errorBody（error-handling.ts）と同じ。
	tests := []struct {
		status int
		Code   apischema.ServerErrorCode
		want   string
	}{
		{500, CodeInternal, ServerMessage},
		{503, CodeOverloaded, OverloadedMessage},
		{404, CodeNotFound, FallbackClientMessage},
	}
	for _, tt := range tests {
		got := newErrorBody(tt.status, tt.Code, "req-1")
		if got != (apischema.ServerError{Error: tt.want, Code: tt.Code, ReqID: "req-1"}) {
			t.Errorf("newErrorBody(%d, %q) = %+v", tt.status, tt.Code, got)
		}
	}
}

func TestPathID(t *testing.T) {
	// Node の idParamsSchema と同じ境目（routes/params.test.ts）。
	tests := []struct {
		Raw    string
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
		t.Run(tt.Raw, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.SetPathValue("id", tt.Raw)
			res := httptest.NewRecorder()

			got, ok := PathID(res, req, "id")
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("pathID(%q) = (%d, %v), want (%d, %v)", tt.Raw, got, ok, tt.want, tt.wantOK)
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
