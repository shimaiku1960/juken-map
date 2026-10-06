package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

// テストの補助。main のテストにも同じもの（router_support_test.go・helpers_test.go）があるが、
// _test.go はパッケージをまたいで import できないので、ここにも置く。

// fakeSessions は DB の代わりに、Cookie の値で決まったセッションを返す。
func fakeSessions(sessions map[string]*Session) SessionLoader {
	return func(r *http.Request) (*Session, error) {
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

var testSessions = map[string]*Session{
	"alice": {UserID: "u1", Email: "alice@example.com", Role: "user"},
	"admin": {UserID: "u2", Email: "admin@example.com", Role: "admin", TwoFactorVerified: true},
	// 管理者だが、2段階認証を通していないセッション（Google / GitHub でのログインなど）
	"admin-no-2fa": {UserID: "u5", Email: "admin@example.com", Role: "admin"},
	"demo":         {UserID: "u3", Email: DemoEmail, Role: "user"},
	"banned":       {UserID: "u4", Email: "banned@example.com", Role: "user", Banned: true},
}

// newTestRouter は入口の種類ごとに1本ずつルートを持つルーター。ハンドラまで来たら 200 と利用者 ID を返す。
func newTestRouter() *Router {
	rt := NewRouter(fakeSessions(testSessions))
	reached := func(w http.ResponseWriter, r *http.Request, s *Session) {
		WriteJSON(w, http.StatusOK, map[string]string{"userId": s.UserID})
	}
	rt.Public("GET /api/public", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	rt.User("GET /api/mine", reached)
	rt.User("POST /api/mine", reached)
	rt.User("DELETE /api/mine/{id}", reached)
	rt.Admin("GET /api/admin/thing", reached)
	rt.Admin("POST /api/admin/thing", reached)
	rt.AnonymousWrite("POST /api/report", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return rt
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

func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("JSON として読めない: %v（%q）", err, s)
	}
	return v
}
