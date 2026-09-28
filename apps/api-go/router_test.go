package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeSessions は DB の代わりに、Cookie の値で決まったセッションを返す。
func fakeSessions(sessions map[string]*session) sessionLoader {
	return func(r *http.Request) (*session, error) {
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

var testSessions = map[string]*session{
	"alice": {UserID: "u1", Email: "alice@example.com", Role: "user"},
	"admin": {UserID: "u2", Email: "admin@example.com", Role: "admin", TwoFactorVerified: true},
	// 管理者だが、2段階認証を通していないセッション（Google / GitHub でのログインなど）
	"admin-no-2fa": {UserID: "u5", Email: "admin@example.com", Role: "admin"},
	"demo":         {UserID: "u3", Email: demoEmail, Role: "user"},
	"banned":       {UserID: "u4", Email: "banned@example.com", Role: "user", Banned: true},
}

// newTestRouter は入口の種類ごとに1本ずつルートを持つルーター。ハンドラまで来たら 200 と利用者 ID を返す。
func newTestRouter() *router {
	rt := newRouter(fakeSessions(testSessions))
	reached := func(w http.ResponseWriter, r *http.Request, s *session) {
		writeJSON(w, http.StatusOK, map[string]string{"userId": s.UserID})
	}
	rt.public("GET /api/public", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	rt.user("GET /api/mine", reached)
	rt.user("POST /api/mine", reached)
	rt.user("DELETE /api/mine/{id}", reached)
	rt.admin("GET /api/admin/thing", reached)
	rt.admin("POST /api/admin/thing", reached)
	return rt
}

func TestRouterAccess(t *testing.T) {
	rt := newTestRouter()
	tests := []struct {
		name       string
		method     string
		path       string
		as         string // Cookie の値。空なら未ログイン
		wantStatus int
		wantBody   string
	}{
		{"公開は未ログインでも通る", "GET", "/api/public", "", 200, `{"ok":true}`},

		{"user: 未ログインは 401", "GET", "/api/mine", "", 401, `{"error":"Unauthorized"}`},
		{"user: 知らないセッションは 401", "GET", "/api/mine", "nobody", 401, `{"error":"Unauthorized"}`},
		{"user: ログイン済みは通る", "GET", "/api/mine", "alice", 200, `{"userId":"u1"}`},
		{"user: 停止中は 403", "GET", "/api/mine", "banned", 403, `{"error":"このアカウントは利用を停止されています。"}`},
		{"user: デモの読み取りは通る", "GET", "/api/mine", "demo", 200, `{"userId":"u3"}`},
		{"user: デモの HEAD は読み取り", "HEAD", "/api/mine", "demo", 200, ""},
		{"user: デモの POST は 403", "POST", "/api/mine", "demo", 403, `{"error":"デモアカウントは閲覧専用です"}`},
		{"user: デモの DELETE は 403", "DELETE", "/api/mine/1", "demo", 403, `{"error":"デモアカウントは閲覧専用です"}`},
		{"user: デモ以外の POST は通る", "POST", "/api/mine", "alice", 200, `{"userId":"u1"}`},

		{"admin: 未ログインは 401", "GET", "/api/admin/thing", "", 401, `{"error":"Unauthorized"}`},
		{"admin: 一般の利用者は 403", "GET", "/api/admin/thing", "alice", 403, `{"error":"Forbidden"}`},
		{"admin: デモも 403", "POST", "/api/admin/thing", "demo", 403, `{"error":"Forbidden"}`},
		{"admin: 管理者は通る", "POST", "/api/admin/thing", "admin", 200, `{"userId":"u2"}`},
		{"admin: 2段階認証を通していない管理者は 403", "GET", "/api/admin/thing", "admin-no-2fa", 403,
			`{"code":"TWO_FACTOR_REQUIRED","error":"管理画面を開くには、2段階認証を通してログインしてください。"}`},

		// Fastify はメソッド違いも 404 にする。ServeMux の既定の 405 にならないこと。
		{"メソッド違いは 404", "PUT", "/api/mine", "alice", 404, ""},
		{"存在しないパスは 404", "GET", "/api/nothing", "", 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.as != "" {
				req.AddCookie(&http.Cookie{Name: "test", Value: tt.as})
			}
			res := httptest.NewRecorder()
			rt.ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d（本文 %s）", res.Code, tt.wantStatus, res.Body)
			}
			if tt.wantBody != "" {
				assertJSONEqual(t, res.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRouterSessionError(t *testing.T) {
	// セッションが読めない（DB が落ちている）ときは、未ログイン扱いにせず 500 にする。
	// 401 にすると、画面はログイン画面へ送ってしまい、原因がログにも残らない。
	rt := newTestRouter()
	req := httptest.NewRequest("GET", "/api/mine", nil)
	req.AddCookie(&http.Cookie{Name: "test", Value: "db-down"})
	res := httptest.NewRecorder()
	rt.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.Code)
	}
	body := decodeJSON(t, res.Body.String())
	if body["code"] != codeInternal || body["error"] != serverMessage {
		t.Errorf("本文 = %v", body)
	}
}

func TestRouterRoutes(t *testing.T) {
	// 登録したルートは、入口の種類と一緒に一覧で読める。
	got := newTestRouter().routes
	want := []routeEntry{
		{"GET /api/public", accessPublic},
		{"GET /api/mine", accessUser},
		{"POST /api/mine", accessUser},
		{"DELETE /api/mine/{id}", accessUser},
		{"GET /api/admin/thing", accessAdmin},
		{"POST /api/admin/thing", accessAdmin},
	}
	if len(got) != len(want) {
		t.Fatalf("routes = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("routes[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRouterRejectsPatternWithoutMethod(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("メソッドの無いルートを登録できてしまった")
		}
	}()
	newRouter(fakeSessions(nil)).user("/api/mine", nil)
}

func TestRouteLabel(t *testing.T) {
	tests := map[string]string{
		"/api/dashboard":                 "/api/dashboard",
		"/api/study-logs/{id}":           "/api/study-logs/:id",
		"/api/goals/{goalId}/items/{id}": "/api/goals/:goalId/items/:id",
		"/api/files/{path...}":           "/api/files/:path",
	}
	for path, want := range tests {
		if got := routeLabel(path); got != want {
			t.Errorf("routeLabel(%q) = %q, want %q", path, got, want)
		}
	}
}
