package httpx

import (
	"net/http/httptest"
	"testing"
)

// 期待値は、Node の Zod（profileSchema と validationErrorBody）に同じ入力を通した結果（2026-09-30、zod 4.5.4）。

func TestObjectInputStopsAtFirstIssue(t *testing.T) {
	// Zod と同じく、スキーマに書いた順で最初の1件だけを返す。後ろの項目は読まずにゼロ値を返す。
	in := ReadObject(map[string]any{"a": "x", "b": "y"})
	if in.Boolean("a") || in.Boolean("b") {
		t.Fatal("弾いた項目や、その後ろの項目が true になった")
	}
	res := httptest.NewRecorder()
	in.Reject(res)
	assertJSONEqual(t, res.Body.String(),
		`{"error":"Invalid input: expected boolean, received string","code":"invalid_type","field":"a"}`)
}
