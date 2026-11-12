// Package validationtest はモデルの入力の決まりを確かめるテストの補助。
// モデルは httpx を知らないので（docs/architecture.md の決まり2）、httpxtest の代わりにこれを使う。
package validationtest

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// Parse は本文を入口と同じく読む（数は json.Number）。
func Parse(t *testing.T, body string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Bad は期待する誤り（コントローラーはこれをそのまま 400 の error・code・field にする）。
// field が "" の誤りは、応答の field が null になる。
func Bad(message, code, field string) string { return code + " " + field + " " + message }

// CheckIssue は want が空なら err が入力の誤りでないこと、そうでなければ err が want の誤りであることを見る。
// 入力の決まりを通ったあとの誤り（DB が無いなど）は、want が空なら気にしない。
func CheckIssue(t *testing.T, err error, want string) {
	t.Helper()
	var v *validation.Issue
	isIssue := errors.As(err, &v)
	if want == "" {
		if isIssue {
			t.Fatalf("弾かれた: %v", err)
		}
		return
	}
	if !isIssue {
		t.Fatalf("誤りが *validation.Issue でない: %v", err)
	}
	if got := Bad(v.Message, v.Code, v.Field); got != want {
		t.Errorf("誤り = %q, want %q", got, want)
	}
}
