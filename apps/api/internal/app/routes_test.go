package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/auth"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/blog"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/line"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

func TestRegisteredRoutesAreRateLimited(t *testing.T) {
	// 本番と同じルートの一覧で、ログインして呼ぶ入口（user・admin）がすべて回数制限を通ることを確かめる。
	// 札を先に使い切っておき、どのルートもハンドラまで来ずに 429 になるかを見る。
	// ログインの入口（auth）は internal/feature/auth/throttle.go が IP とアカウントで別に数えるので対象外。
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	auth.RegisterRoutes(rt, auth.New(nil, auth.Config{}))
	registerRoutes(rt, nil, allJobs(), line.Config{}, blog.WebhookConfig{})
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

// TestAdminRoutesRejectNonAdmins は、管理者用として登録した全ルートが、管理者＋2段階認証の
// セッション以外を断ることを、ルートの一覧から自動で確かめる（セキュリティ基準 A5）。
// ルートを足しても、このテストに書き足さなくても確かめられる。ハンドラまで来ると DB が nil で
// panic するので、断れていなければテストが落ちる。
func TestAdminRoutesRejectNonAdmins(t *testing.T) {
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	registerRoutes(rt, nil, allJobs(), line.Config{}, blog.WebhookConfig{})

	tests := []struct {
		as         string
		wantStatus int
		wantBody   string
	}{
		{"", 401, `{"error":"Unauthorized"}`},
		{"alice", 403, `{"error":"Forbidden"}`},
		{"demo", 403, `{"error":"Forbidden"}`},
		{"banned", 403, `{"error":"このアカウントは利用を停止されています。"}`},
		{"admin-no-2fa", 403, `{"code":"TWO_FACTOR_REQUIRED","error":"管理画面を開くには、2段階認証を通してログインしてください。"}`},
	}
	admins := 0
	for _, route := range rt.Routes {
		if route.Access != httpx.AccessAdmin {
			continue
		}
		admins++
		method, path, _ := strings.Cut(route.Pattern, " ")
		path = httpx.PathParam.ReplaceAllString(path, "1")
		for _, tt := range tests {
			req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			if tt.as != "" {
				req.AddCookie(&http.Cookie{Name: "test", Value: tt.as})
			}
			rec := httptest.NewRecorder()
			rt.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s を %q で: status = %d, want %d", route.Pattern, tt.as, rec.Code, tt.wantStatus)
				continue
			}
			httpxtest.AssertJSONEqual(t, rec.Body.String(), tt.wantBody)
		}
	}
	if admins == 0 {
		t.Fatal("管理者用のルートが1本も無い")
	}
}
