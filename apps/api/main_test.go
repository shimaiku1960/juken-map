package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

func TestRegisteredRoutes(t *testing.T) {
	// Go が受け持つルートと入口の種類。
	// ルートを足したらここに1行足す（nginx の振り分けも）。
	// 一覧が変わるとこのテストが落ちるので、入口の種類を取り違えたまま足すことはできない。
	want := []httpx.RouteEntry{
		{Pattern: "GET /api/auth/session", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/sign-up", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/sign-in", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/sign-out", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/verify-email", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/verify-email/resend", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/password/forgot", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/password/reset", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/password/change", Access: httpx.AccessAuth},
		{Pattern: "GET /api/auth/accounts", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/delete-account", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/mfa/setup", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/mfa/confirm", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/mfa/verify", Access: httpx.AccessAuth},
		{Pattern: "POST /api/auth/oauth/{provider}", Access: httpx.AccessAuth},
		{Pattern: "GET /api/auth/callback/{provider}", Access: httpx.AccessAuth},
		{Pattern: "GET /api/blog", Access: httpx.AccessPublic},
		{Pattern: "GET /api/blog/{id}", Access: httpx.AccessPublic},
		{Pattern: "GET /api/health", Access: httpx.AccessPublic},
		{Pattern: "GET /api/dashboard", Access: httpx.AccessUser},
		{Pattern: "GET /api/study-logs", Access: httpx.AccessUser},
		{Pattern: "GET /api/study-logs/daily", Access: httpx.AccessUser},
		{Pattern: "GET /api/study-plans", Access: httpx.AccessUser},
		{Pattern: "POST /api/study-logs", Access: httpx.AccessUser},
		{Pattern: "PATCH /api/study-logs/{id}", Access: httpx.AccessUser},
		{Pattern: "DELETE /api/study-logs/{id}", Access: httpx.AccessUser},
		{Pattern: "POST /api/study-plans", Access: httpx.AccessUser},
		{Pattern: "PATCH /api/study-plans/{id}", Access: httpx.AccessUser},
		{Pattern: "DELETE /api/study-plans/{id}", Access: httpx.AccessUser},
		{Pattern: "POST /api/study-plans/{id}/complete", Access: httpx.AccessUser},
		{Pattern: "GET /api/goals", Access: httpx.AccessUser},
		{Pattern: "GET /api/goals/first-choice", Access: httpx.AccessUser},
		{Pattern: "POST /api/goals", Access: httpx.AccessUser},
		{Pattern: "PUT /api/goals/{id}", Access: httpx.AccessUser},
		{Pattern: "PATCH /api/goals/{id}", Access: httpx.AccessUser},
		{Pattern: "DELETE /api/goals/{id}", Access: httpx.AccessUser},
		{Pattern: "GET /api/textbooks", Access: httpx.AccessUser},
		{Pattern: "GET /api/textbook-masters", Access: httpx.AccessUser},
		{Pattern: "POST /api/textbooks", Access: httpx.AccessUser},
		{Pattern: "PATCH /api/textbooks/{id}", Access: httpx.AccessUser},
		{Pattern: "GET /api/notification-preferences", Access: httpx.AccessUser},
		{Pattern: "PUT /api/notification-preferences", Access: httpx.AccessUser},
		{Pattern: "PUT /api/profile", Access: httpx.AccessUser},
		{Pattern: "GET /api/universities", Access: httpx.AccessUser},
		{Pattern: "GET /api/universities/{id}", Access: httpx.AccessUser},
		{Pattern: "POST /api/analytics/registration", Access: httpx.AccessUser},
		{Pattern: "POST /api/csp-report", Access: httpx.AccessAnonymousWrite},
		{Pattern: "GET /api/line/connection", Access: httpx.AccessUser},
		{Pattern: "DELETE /api/line/connection", Access: httpx.AccessUser},
		{Pattern: "POST /api/line/account-link", Access: httpx.AccessUser},
		{Pattern: "GET /api/line/oauth/start", Access: httpx.AccessOAuth},
		{Pattern: "GET /api/line/oauth/callback", Access: httpx.AccessOAuth},
		{Pattern: "POST /api/line/webhook", Access: httpx.AccessWebhook},
		{Pattern: "GET /line/settings", Access: httpx.AccessPublic},
		{Pattern: "POST /api/webhooks/microcms", Access: httpx.AccessWebhook},
		{Pattern: "GET /api/admin/overview", Access: httpx.AccessAdmin},
		{Pattern: "GET /api/admin/users", Access: httpx.AccessAdmin},
		{Pattern: "POST /api/admin/users/{id}/ban", Access: httpx.AccessAdmin},
		{Pattern: "POST /api/admin/users/{id}/unban", Access: httpx.AccessAdmin},
		{Pattern: "DELETE /api/admin/users/{id}", Access: httpx.AccessAdmin},
		{Pattern: "GET /api/admin/universities", Access: httpx.AccessAdmin},
		{Pattern: "POST /api/admin/universities", Access: httpx.AccessAdmin},
		{Pattern: "GET /api/admin/universities/{id}", Access: httpx.AccessAdmin},
		{Pattern: "PATCH /api/admin/universities/{id}", Access: httpx.AccessAdmin},
		{Pattern: "DELETE /api/admin/universities/{id}", Access: httpx.AccessAdmin},
		{Pattern: "GET /api/admin/tags", Access: httpx.AccessAdmin},
		{Pattern: "POST /api/admin/faculties", Access: httpx.AccessAdmin},
		{Pattern: "PATCH /api/admin/faculties/{id}", Access: httpx.AccessAdmin},
		{Pattern: "DELETE /api/admin/faculties/{id}", Access: httpx.AccessAdmin},
		{Pattern: "GET /api/admin/textbook-masters", Access: httpx.AccessAdmin},
		{Pattern: "POST /api/admin/textbook-masters", Access: httpx.AccessAdmin},
		{Pattern: "PATCH /api/admin/textbook-masters/{id}", Access: httpx.AccessAdmin},
		{Pattern: "DELETE /api/admin/textbook-masters/{id}", Access: httpx.AccessAdmin},
		{Pattern: "POST /api/cron/daily-study-notifications", Access: httpx.AccessJob},
		{Pattern: "GET /api/sim/state", Access: httpx.AccessJob},
		{Pattern: "POST /api/sim/users", Access: httpx.AccessJob},
		{Pattern: "PATCH /api/sim/users/{seq}", Access: httpx.AccessJob},
	}

	// ハンドラは呼ばないので、DB は nil のままでよい。
	rt := httpx.NewRouter(httpxtest.FakeSessions(nil))
	registerAuthRoutes(rt, newAuthHandlers(nil, authConfig{}))
	registerBlogRoutes(rt, blogConfig{})
	registerRoutes(rt, nil, jobConfig{simulationEnabled: true}, lineConfig{}, microcmsWebhookConfig{})

	if len(rt.Routes) != len(want) {
		t.Fatalf("routes = %v\nwant %v", rt.Routes, want)
	}
	for i := range want {
		if rt.Routes[i] != want[i] {
			t.Errorf("routes[%d] = %v, want %v", i, rt.Routes[i], want[i])
		}
	}
}

func TestRegisteredWritesRejectCrossSite(t *testing.T) {
	// CSRF 対策（セキュリティ基準 D3）を、本番と同じルートの一覧で確かめる。
	// Cookie で認証する書き込み（入口が user・admin の GET 以外）は、別のサイトから送られたらハンドラまで来ずに 403。
	// それ以外の入口は Cookie を読まないので CSRF の対象にならない。Cookie を読む入口（public・oauth）に
	// 書き込みのルートを足したら、ここで落ちる（足すなら user・admin で登録する）。
	// 仕組みそのもの（同じサイト・Origin と Host の比較・curl）は internal/httpx/router_test.go の TestRouterCrossOrigin。
	//
	// 認証の入口（auth）はセッションが無くても呼べるが、書き込みは同じく断る。ログイン・登録・ログアウト・
	// 2段階認証の確認を別のサイトから送らせない（ログインの CSRF。認証基準 10 の D2）。
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	registerAuthRoutes(rt, newAuthHandlers(nil, authConfig{}))
	registerBlogRoutes(rt, blogConfig{})
	registerRoutes(rt, nil, jobConfig{simulationEnabled: true}, lineConfig{}, microcmsWebhookConfig{})

	checked := 0
	for _, route := range rt.Routes {
		method, path, _ := strings.Cut(route.Pattern, " ")
		if method == http.MethodGet {
			continue
		}
		switch route.Access {
		case httpx.AccessUser, httpx.AccessAdmin, httpx.AccessAuth:
		case httpx.AccessWebhook, httpx.AccessJob, httpx.AccessAnonymousWrite:
			// 署名・共有トークンで守るか、書き込めても害が無い入口。Cookie は読まない。
			continue
		default:
			t.Errorf("%s: Cookie を読む入口（%s）に書き込みがある。user か admin で登録する", route.Pattern, route.Access)
			continue
		}
		target := httpx.PathParam.ReplaceAllString(path, "1")
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
				httpxtest.AssertJSONEqual(t, res.Body.String(), `{"error":"`+httpx.CrossOriginMessage+`"}`)
			})
		}
		checked++
	}
	// 一覧が空になって何も確かめずに通る、ということが無いように。
	if checked < 20 {
		t.Fatalf("確かめた書き込みのルートが %d 本しかない", checked)
	}
}
