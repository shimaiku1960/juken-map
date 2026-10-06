package httpx

import (
	"reflect"
	"testing"
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
		if got := ParseQuery(tt.raw); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseQuery(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}
