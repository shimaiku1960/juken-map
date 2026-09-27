package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// テストだけで使う道具。ファイル名が _test.go で終わるので、本番のビルドには入らない。

func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("JSON として読めない: %v（%q）", err, s)
	}
	return v
}

// assertJSONEqual はキーの順番や空白の違いを無視して、JSON として同じかを比べる。
func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("JSON として読めない: %v（%q）", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("期待値が JSON として読めない: %v（%q）", err, want)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("本文 = %s, want %s", got, want)
	}
}
