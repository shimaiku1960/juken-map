// Package httpxtest は入口（internal/httpx のルーター）と入力チェックを使うテストの補助。
// _test.go の補助はほかのパッケージから import できないので、feature のテストが共通で使うものをここに置く（JUK-156）。
// 本番のコードからは import しない。
package httpxtest

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// FakeSessions は DB の代わりに、Cookie の値で決まったセッションを返す。
func FakeSessions(sessions map[string]*httpx.Session) httpx.SessionLoader {
	return func(r *http.Request) (*httpx.Session, error) {
		c, err := r.Cookie("test")
		if err != nil {
			return nil, nil
		}
		if c.Value == "db-down" {
			return nil, errors.New("connection refused")
		}
		return sessions[c.Value], nil
	}
}

// Sessions は FakeSessions に渡すセッション。Cookie の test の値（alice・admin・demo など）で選ぶ。
var Sessions = map[string]*httpx.Session{
	"alice": {UserID: "u1", Email: "alice@example.com", Role: "user"},
	"admin": {UserID: "u2", Email: "admin@example.com", Role: "admin", TwoFactorVerified: true},
	// 管理者だが、2段階認証を通していないセッション（Google / GitHub でのログインなど）
	"admin-no-2fa": {UserID: "u5", Email: "admin@example.com", Role: "admin"},
	"demo":         {UserID: "u3", Email: httpx.DemoEmail, Role: "user"},
	"banned":       {UserID: "u4", Email: "banned@example.com", Role: "user", Banned: true},
}

// DecodeJSON は応答の本文を JSON のオブジェクトとして読む。
func DecodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("JSON として読めない: %v（%q）", err, s)
	}
	return v
}

// AssertJSONEqual はキーの順番や空白の違いを無視して、JSON として同じかを比べる。
func AssertJSONEqual(t *testing.T, got, want string) {
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

// IssueJSON は入力チェックで弾いたときの 400 の本文（Zod の最初の issue と同じ形）。
func IssueJSON(message, code, field string) string {
	return `{"error":"` + message + `","code":"` + code + `","field":"` + field + `"}`
}

// CheckIssue は入力チェックの結果を見る。want が空なら通ること、そうでなければ弾いた本文が want であること。
func CheckIssue(t *testing.T, in *httpx.ObjectInput, want string) {
	t.Helper()
	res := httptest.NewRecorder()
	rejected := in.Reject(res)
	if want == "" {
		if rejected {
			t.Fatalf("弾かれた: %s", res.Body)
		}
		return
	}
	if !rejected {
		t.Fatal("通ってしまった")
	}
	AssertJSONEqual(t, res.Body.String(), want)
}

// ParseBody はリクエストの本文を、入口と同じ読み方（httpx.ParseJSON）で読む。
func ParseBody(t *testing.T, body string) any {
	t.Helper()
	v, err := httpx.ParseJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
