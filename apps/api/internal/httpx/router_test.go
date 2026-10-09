package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

		{"anonymous-write: 未ログインでも通る", "POST", "/api/report", "", 204, ""},
		{"anonymous-write: セッションを見ないので、停止中でも通る", "POST", "/api/report", "banned", 204, ""},
		{"anonymous-write: 読み取りは無い", "GET", "/api/report", "", 404, ""},

		// メソッド違いも 404 にする（以前の Fastify と同じ）。ServeMux の既定の 405 にならないこと。
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

func TestRouterCrossOrigin(t *testing.T) {
	// Cookie で認証する書き込みは、別のサイトから送られたら断る（CSRF 対策）。
	rt := newTestRouter()
	tests := []struct {
		name       string
		method     string
		path       string
		header     map[string]string
		wantStatus int
	}{
		{"同じサイトの画面からの POST は通る", "POST", "/api/mine", map[string]string{"Sec-Fetch-Site": "same-origin"}, 200},
		{"別のサイトからの POST は 403", "POST", "/api/mine", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"同じサイトの別オリジン（サブドメイン）も 403", "POST", "/api/mine", map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		{"利用者が直接開いた（none）は通る", "POST", "/api/mine", map[string]string{"Sec-Fetch-Site": "none"}, 200},
		{"Sec-Fetch-Site が無く、Origin が Host と同じなら通る", "POST", "/api/mine",
			map[string]string{"Origin": "https://juken-map.com", "Host": "juken-map.com"}, 200},
		{"Sec-Fetch-Site が無く、Origin が別なら 403", "DELETE", "/api/mine/1",
			map[string]string{"Origin": "https://evil.example", "Host": "juken-map.com"}, 403},
		{"どちらも無い（curl など）は通る", "POST", "/api/mine", nil, 200},
		{"別のサイトからでも読み取りは通る", "GET", "/api/mine", map[string]string{"Sec-Fetch-Site": "cross-site"}, 200},
		{"管理者の書き込みも同じ", "POST", "/api/admin/thing", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.AddCookie(&http.Cookie{Name: "test", Value: "admin"})
			for k, v := range tt.header {
				if k == "Host" {
					req.Host = v
					continue
				}
				req.Header.Set(k, v)
			}
			res := httptest.NewRecorder()
			rt.ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d（本文 %s）", res.Code, tt.wantStatus, res.Body)
			}
			if tt.wantStatus == 403 {
				assertJSONEqual(t, res.Body.String(), `{"error":"別のサイトからの書き込みは受け付けません"}`)
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
	if body["code"] != string(CodeInternal) || body["error"] != ServerMessage {
		t.Errorf("本文 = %v", body)
	}
}

func TestRouterRoutes(t *testing.T) {
	// 登録したルートは、入口の種類と一緒に一覧で読める。
	got := newTestRouter().Routes
	want := []RouteEntry{
		{"GET /api/public", AccessPublic},
		{"GET /api/mine", AccessUser},
		{"POST /api/mine", AccessUser},
		{"DELETE /api/mine/{id}", AccessUser},
		{"GET /api/admin/thing", AccessAdmin},
		{"POST /api/admin/thing", AccessAdmin},
		{"POST /api/report", AccessAnonymousWrite},
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
	NewRouter(fakeSessions(nil)).User("/api/mine", nil)
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
