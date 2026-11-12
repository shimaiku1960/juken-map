package httpx

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

func TestRejectWritesFirstIssue(t *testing.T) {
	in := validation.ReadObject(map[string]any{"a": "x"})
	in.Boolean("a")
	res := httptest.NewRecorder()
	if !Reject(res, in) {
		t.Fatal("issue があるのに Reject が false を返した")
	}
	if res.Code != 400 {
		t.Fatalf("status = %d, want 400", res.Code)
	}
	assertJSONEqual(t, res.Body.String(),
		`{"error":"Invalid input: expected boolean, received string","code":"invalid_type","field":"a"}`)
}

func TestRejectPassesWithoutIssue(t *testing.T) {
	in := validation.ReadObject(map[string]any{"a": true})
	in.Boolean("a")
	if Reject(httptest.NewRecorder(), in) {
		t.Fatal("issue が無いのに Reject が true を返した")
	}
}

func TestRejectIssue(t *testing.T) {
	res := httptest.NewRecorder()
	// 包まれていても見つける。field が無ければ null。
	err := fmt.Errorf("op: %w", &validation.Issue{Code: "custom", Message: "だめ"})
	if !RejectIssue(res, err) {
		t.Fatal("入力の誤りなのに RejectIssue が false を返した")
	}
	if res.Code != 400 {
		t.Fatalf("status = %d, want 400", res.Code)
	}
	assertJSONEqual(t, res.Body.String(), `{"error":"だめ","code":"custom","field":null}`)

	if RejectIssue(httptest.NewRecorder(), errors.New("db down")) {
		t.Fatal("入力の誤りでないのに RejectIssue が true を返した")
	}
}
