package main

// 入口（internal/httpx のルーター）を使うテストの補助。internal/httpx の helpers_test.go にも同じものがある。

import (
	"errors"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// fakeSessions は DB の代わりに、Cookie の値で決まったセッションを返す。
func fakeSessions(sessions map[string]*httpx.Session) httpx.SessionLoader {
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

var testSessions = map[string]*httpx.Session{
	"alice": {UserID: "u1", Email: "alice@example.com", Role: "user"},
	"admin": {UserID: "u2", Email: "admin@example.com", Role: "admin", TwoFactorVerified: true},
	// 管理者だが、2段階認証を通していないセッション（Google / GitHub でのログインなど）
	"admin-no-2fa": {UserID: "u5", Email: "admin@example.com", Role: "admin"},
	"demo":         {UserID: "u3", Email: httpx.DemoEmail, Role: "user"},
	"banned":       {UserID: "u4", Email: "banned@example.com", Role: "user", Banned: true},
}

// newTestRouter は入口の種類ごとに1本ずつルートを持つルーター。ハンドラまで来たら 200 と利用者 ID を返す。
func newTestRouter() *httpx.Router {
	rt := httpx.NewRouter(fakeSessions(testSessions))
	reached := func(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"userId": s.UserID})
	}
	rt.Public("GET /api/public", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
