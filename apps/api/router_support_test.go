package main

// 入口（internal/httpx のルーター）を使うテストの補助。偽のセッションなどの共通の補助は internal/httpx/httpxtest にある。

import (
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

// newTestRouter は入口の種類ごとに1本ずつルートを持つルーター。ハンドラまで来たら 200 と利用者 ID を返す。
func newTestRouter() *httpx.Router {
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
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
