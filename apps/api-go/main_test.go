package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisteredRoutes(t *testing.T) {
	// Go が受け持つルートと入口の種類。
	// ルートを足したらここに1行足す（nginx の振り分けも）。
	// 一覧が変わるとこのテストが落ちるので、入口の種類を取り違えたまま足すことはできない。
	want := []routeEntry{
		{"GET /api/auth/session", accessAuth},
		{"POST /api/auth/sign-up", accessAuth},
		{"POST /api/auth/sign-in", accessAuth},
		{"POST /api/auth/sign-out", accessAuth},
		{"POST /api/auth/verify-email", accessAuth},
		{"POST /api/auth/verify-email/resend", accessAuth},
		{"POST /api/auth/password/forgot", accessAuth},
		{"POST /api/auth/password/reset", accessAuth},
		{"POST /api/auth/password/change", accessAuth},
		{"GET /api/auth/accounts", accessAuth},
		{"POST /api/auth/mfa/setup", accessAuth},
		{"POST /api/auth/mfa/confirm", accessAuth},
		{"POST /api/auth/mfa/verify", accessAuth},
		{"POST /api/auth/oauth/{provider}", accessAuth},
		{"GET /api/auth/callback/{provider}", accessAuth},
		{"GET /api/health", accessPublic},
		{"GET /api/dashboard", accessUser},
		{"GET /api/study-logs", accessUser},
		{"GET /api/study-logs/daily", accessUser},
		{"GET /api/study-plans", accessUser},
		{"POST /api/study-logs", accessUser},
		{"PATCH /api/study-logs/{id}", accessUser},
		{"DELETE /api/study-logs/{id}", accessUser},
		{"POST /api/study-plans", accessUser},
		{"PATCH /api/study-plans/{id}", accessUser},
		{"DELETE /api/study-plans/{id}", accessUser},
		{"POST /api/study-plans/{id}/complete", accessUser},
		{"GET /api/goals", accessUser},
		{"GET /api/goals/first-choice", accessUser},
		{"POST /api/goals", accessUser},
		{"PUT /api/goals/{id}", accessUser},
		{"PATCH /api/goals/{id}", accessUser},
		{"DELETE /api/goals/{id}", accessUser},
		{"GET /api/textbooks", accessUser},
		{"GET /api/textbook-masters", accessUser},
		{"POST /api/textbooks", accessUser},
		{"PATCH /api/textbooks/{id}", accessUser},
		{"GET /api/notification-preferences", accessUser},
		{"PUT /api/notification-preferences", accessUser},
		{"PUT /api/profile", accessUser},
		{"GET /api/universities", accessUser},
		{"GET /api/universities/{id}", accessUser},
		{"POST /api/analytics/registration", accessUser},
		{"POST /api/csp-report", accessAnonymousWrite},
		{"GET /api/line/connection", accessUser},
		{"DELETE /api/line/connection", accessUser},
		{"POST /api/line/account-link", accessUser},
		{"GET /api/line/oauth/start", accessOAuth},
		{"GET /api/line/oauth/callback", accessOAuth},
		{"POST /api/line/webhook", accessWebhook},
		{"POST /api/webhooks/microcms", accessWebhook},
		{"GET /api/admin/overview", accessAdmin},
		{"GET /api/admin/users", accessAdmin},
		{"POST /api/admin/users/{id}/ban", accessAdmin},
		{"POST /api/admin/users/{id}/unban", accessAdmin},
		{"DELETE /api/admin/users/{id}", accessAdmin},
		{"GET /api/admin/universities", accessAdmin},
		{"POST /api/admin/universities", accessAdmin},
		{"GET /api/admin/universities/{id}", accessAdmin},
		{"PATCH /api/admin/universities/{id}", accessAdmin},
		{"DELETE /api/admin/universities/{id}", accessAdmin},
		{"GET /api/admin/tags", accessAdmin},
		{"POST /api/admin/faculties", accessAdmin},
		{"PATCH /api/admin/faculties/{id}", accessAdmin},
		{"DELETE /api/admin/faculties/{id}", accessAdmin},
		{"GET /api/admin/textbook-masters", accessAdmin},
		{"POST /api/admin/textbook-masters", accessAdmin},
		{"PATCH /api/admin/textbook-masters/{id}", accessAdmin},
		{"DELETE /api/admin/textbook-masters/{id}", accessAdmin},
		{"POST /api/cron/daily-study-notifications", accessJob},
		{"GET /api/sim/state", accessJob},
		{"POST /api/sim/users", accessJob},
		{"PATCH /api/sim/users/{seq}", accessJob},
	}

	// ハンドラは呼ばないので、DB は nil のままでよい。
	rt := newRouter(fakeSessions(nil))
	registerAuthRoutes(rt, newAuthHandlers(nil, authConfig{}))
	registerRoutes(rt, nil, jobConfig{simulationEnabled: true}, lineConfig{}, microcmsWebhookConfig{})

	if len(rt.routes) != len(want) {
		t.Fatalf("routes = %v\nwant %v", rt.routes, want)
	}
	for i := range want {
		if rt.routes[i] != want[i] {
			t.Errorf("routes[%d] = %v, want %v", i, rt.routes[i], want[i])
		}
	}
}

func TestRegisteredWritesRejectCrossSite(t *testing.T) {
	// CSRF 対策（セキュリティ基準 D3）を、本番と同じルートの一覧で確かめる。
	// Cookie で認証する書き込み（入口が user・admin の GET 以外）は、別のサイトから送られたらハンドラまで来ずに 403。
	// それ以外の入口は Cookie を読まないので CSRF の対象にならない。Cookie を読む入口（public・oauth）に
	// 書き込みのルートを足したら、ここで落ちる（足すなら user・admin で登録する）。
	// 仕組みそのもの（同じサイト・Origin と Host の比較・curl）は router_test.go の TestRouterCrossOrigin。
	//
	// 認証の入口（auth）はセッションが無くても呼べるが、書き込みは同じく断る。ログイン・登録・ログアウト・
	// 2段階認証の確認を別のサイトから送らせない（ログインの CSRF。認証基準 10 の D2）。
	rt := newRouter(fakeSessions(testSessions))
	registerAuthRoutes(rt, newAuthHandlers(nil, authConfig{}))
	registerRoutes(rt, nil, jobConfig{simulationEnabled: true}, lineConfig{}, microcmsWebhookConfig{})

	checked := 0
	for _, route := range rt.routes {
		method, path, _ := strings.Cut(route.Pattern, " ")
		if method == http.MethodGet {
			continue
		}
		switch route.Access {
		case accessUser, accessAdmin, accessAuth:
		case accessWebhook, accessJob, accessAnonymousWrite:
			// 署名・共有トークンで守るか、書き込めても害が無い入口。Cookie は読まない。
			continue
		default:
			t.Errorf("%s: Cookie を読む入口（%s）に書き込みがある。user か admin で登録する", route.Pattern, route.Access)
			continue
		}
		target := pathParam.ReplaceAllString(path, "1")
		for _, header := range []map[string]string{
			{"Sec-Fetch-Site": "cross-site"},
			// Sec-Fetch-Site を送らない古いブラウザは、Origin と Host を比べて見分ける
			{"Origin": "https://evil.example"},
		} {
			t.Run(route.Pattern+" "+fmt.Sprint(header), func(t *testing.T) {
				req := httptest.NewRequest(method, target, strings.NewReader(`{}`))
				req.Host = "juken-map.com"
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(&http.Cookie{Name: "test", Value: "admin"})
				for k, v := range header {
					req.Header.Set(k, v)
				}
				res := httptest.NewRecorder()
				func() {
					// DB は nil なので、ハンドラまで来たら panic になる。来たこと自体を失敗として出す。
					defer func() {
						if p := recover(); p != nil {
							t.Fatalf("別のサイトからの書き込みがハンドラまで来た: %v", p)
						}
					}()
					rt.ServeHTTP(res, req)
				}()
				if res.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want 403（本文 %s）", res.Code, res.Body)
				}
				assertJSONEqual(t, res.Body.String(), `{"error":"`+crossOriginMessage+`"}`)
			})
		}
		checked++
	}
	// 一覧が空になって何も確かめずに通る、ということが無いように。
	if checked < 20 {
		t.Fatalf("確かめた書き込みのルートが %d 本しかない", checked)
	}
}
