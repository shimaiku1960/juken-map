package validation

import (
	"encoding/json"
	"math"
	"testing"
)

// 期待値は、Node の Zod（profileSchema と validationErrorBody）に同じ入力を通した結果（2026-09-30、zod 4.5.4）。

func TestObjectInputStopsAtFirstIssue(t *testing.T) {
	// Zod と同じく、スキーマに書いた順で最初の1件だけを返す。後ろの項目は読まずにゼロ値を返す。
	in := ReadObject(map[string]any{"a": "x", "b": "y"})
	if in.Boolean("a") || in.Boolean("b") {
		t.Fatal("弾いた項目や、その後ろの項目が true になった")
	}
	want := Issue{Code: "invalid_type", Field: "a", Message: "Invalid input: expected boolean, received string"}
	if in.Issue == nil || *in.Issue != want {
		t.Fatalf("issue = %+v, want %+v", in.Issue, want)
	}
}

func TestJSNumber(t *testing.T) {
	// JSON.parse と同じく、範囲外の数は ±Infinity、小さすぎる数は 0 になる。
	tests := map[string]float64{
		"1": 1, "1.5": 1.5, "1e3": 1000, "-0": math.Copysign(0, -1),
		"1e400": math.Inf(1), "-1e400": math.Inf(-1), "1e-400": 0,
	}
	for in, want := range tests {
		if got := JSNumber(json.Number(in)); got != want || math.Signbit(got) != math.Signbit(want) {
			t.Errorf("JSNumber(%s) = %v, want %v", in, got, want)
		}
	}
}

func TestIsCalendarYMD(t *testing.T) {
	for s, want := range map[string]bool{
		"2026-09-30": true, "2024-02-29": true, "2000-02-29": true, "0000-02-29": true,
		"2026-02-30": false, "2025-02-29": false, "1900-02-29": false, "2025-13-01": false,
		"2025-00-10": false, "2025-04-31": false, "2025-01-00": false,
	} {
		if got := IsCalendarYMD(s); got != want {
			t.Errorf("IsCalendarYMD(%s) = %v, want %v", s, got, want)
		}
	}
}
