package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

func TestRegisteredRoutesAreRateLimited(t *testing.T) {
	// 本番と同じルートの一覧で、ログインして呼ぶ入口（user・admin）がすべて回数制限を通ることを確かめる。
	// 札を先に使い切っておき、どのルートもハンドラまで来ずに 429 になるかを見る。
	// ログインの入口（auth）は auth_throttle.go が IP とアカウントで別に数えるので対象外。
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	registerAuthRoutes(rt, newAuthHandlers(nil, authConfig{}))
	registerRoutes(rt, nil, jobConfig{simulationEnabled: true}, lineConfig{}, microcmsWebhookConfig{})
	rt.UseUp(httpxtest.Sessions["alice"].UserID, httpxtest.Sessions["admin"].UserID)

	checked := 0
	for _, route := range rt.Routes {
		as := ""
		switch route.Access {
		case httpx.AccessUser:
			as = "alice"
		case httpx.AccessAdmin:
			as = "admin"
		default:
			continue
		}
		method, path, _ := strings.Cut(route.Pattern, " ")
		t.Run(route.Pattern, func(t *testing.T) {
			var res *httptest.ResponseRecorder
			func() {
				// DB は nil なので、ハンドラまで来たら panic になる。来たこと自体を失敗として出す。
				defer func() {
					if p := recover(); p != nil {
						t.Fatalf("回数制限を超えたのにハンドラまで来た: %v", p)
					}
				}()
				req := httptest.NewRequest(method, httpx.PathParam.ReplaceAllString(path, "1"), strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(&http.Cookie{Name: "test", Value: as})
				res = httptest.NewRecorder()
				rt.ServeHTTP(res, req)
			}()
			if res.Code != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429（本文 %s）", res.Code, res.Body)
			}
		})
		checked++
	}
	// 一覧が空になって何も確かめずに通る、ということが無いように。
	if checked < 40 {
		t.Fatalf("確かめたルートが %d 本しかない", checked)
	}
}
