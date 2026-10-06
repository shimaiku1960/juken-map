package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

func TestNicknameRule(t *testing.T) {
	const (
		fullwidthSpace = "\u3000"
		bom            = "\ufeff"
		nel            = "\u0085"
		nbsp           = "\u00a0"
		zeroWidthSpace = "\u200b"
		enQuad         = "\u2000"
		emoji          = "\U0001F600"
	)
	typeError := `{"error":"ニックネームは文字列で入力してください","code":"invalid_type","field":"nickname"}`
	tooBig := `{"error":"50文字以内で入力してください","code":"too_big","field":"nickname"}`
	tooSmall := `{"error":"ニックネームは必須です","code":"too_small","field":"nickname"}`

	tests := []struct {
		name  string
		body  any
		want  string // 通ったときの値
		issue string // 弾いたときの 400 の本文
	}{
		{"無い", map[string]any{}, "", typeError},
		{"数", map[string]any{"nickname": 1.0}, "", typeError},
		{"null", map[string]any{"nickname": nil}, "", typeError},
		{"空文字", map[string]any{"nickname": ""}, "", tooSmall},
		{"空白だけは弾く（削ってから min）", map[string]any{"nickname": "   "}, "", tooSmall},
		{"全角スペースだけも弾く", map[string]any{"nickname": fullwidthSpace}, "", tooSmall},
		{"全角スペースを削る", map[string]any{"nickname": fullwidthSpace + "山田" + fullwidthSpace}, "山田", ""},
		{"BOM を削る", map[string]any{"nickname": bom + "山田"}, "山田", ""},
		{"NBSP を削る", map[string]any{"nickname": nbsp + "山田"}, "山田", ""},
		{"U+2000 を削る", map[string]any{"nickname": enQuad + "山田"}, "山田", ""},
		{"U+0085 は削らない", map[string]any{"nickname": nel + "山田"}, nel + "山田", ""},
		{"ゼロ幅スペースは削らない", map[string]any{"nickname": zeroWidthSpace + "山田"}, zeroWidthSpace + "山田", ""},
		{"50文字は通る", map[string]any{"nickname": strings.Repeat("あ", 50)}, strings.Repeat("あ", 50), ""},
		{"51文字は弾く", map[string]any{"nickname": strings.Repeat("あ", 51)}, "", tooBig},
		// 長さはコードポイントで数える。絵文字は UTF-16 では 2 だが 1 と数える
		{"絵文字50個は通る", map[string]any{"nickname": strings.Repeat(emoji, 50)}, strings.Repeat(emoji, 50), ""},
		{"絵文字51個は弾く", map[string]any{"nickname": strings.Repeat(emoji, 51)}, "", tooBig},
		// 長さは削った後に確かめる
		{"前後の空白込みで53文字でも、削って50文字なら通る", map[string]any{"nickname": " " + strings.Repeat("a", 50) + "  "}, strings.Repeat("a", 50), ""},
		{"削っても51文字なら弾く", map[string]any{"nickname": " " + strings.Repeat("a", 51) + " "}, "", tooBig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := httpx.ReadObject(tt.body)
			got := in.String("nickname", nicknameRule)
			res := httptest.NewRecorder()
			rejected := in.Reject(res)

			if tt.issue != "" {
				if !rejected {
					t.Fatalf("通ってしまった（%q）", got)
				}
				httpxtest.AssertJSONEqual(t, res.Body.String(), tt.issue)
				return
			}
			if rejected {
				t.Fatalf("弾かれた: %s", res.Body)
			}
			if got != tt.want {
				t.Errorf("nickname = %q, want %q", got, tt.want)
			}
		})
	}
}
