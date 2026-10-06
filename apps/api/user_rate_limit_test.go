package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
)

// fakeClock はテストで進める時計。
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
}

// use は札を n 回使い、通った回数を返す。
func use(l *userRateLimiter, rule rateLimitRule, key string, n int) int {
	allowed := 0
	for range n {
		if ok, _ := l.allow(rule, key); ok {
			allowed++
		}
	}
	return allowed
}

func TestUserRateLimiterBucket(t *testing.T) {
	clock := newFakeClock()
	l := newUserRateLimiter(clock.now)

	// 満タンから始まり、burst までは続けて通る
	if got := use(l, rateLimitRead, "u1", 120); got != 120 {
		t.Fatalf("続けて通った数 = %d, want 120", got)
	}
	// 尽きたら断り、1つ戻るまでの時間を返す（毎秒2つ戻るので 0.5 秒）
	ok, wait := l.allow(rateLimitRead, "u1")
	if ok || wait != 500*time.Millisecond {
		t.Fatalf("allow = %v, %v, want false, 500ms", ok, wait)
	}
	// 断ったときは札を減らさない（待ってから送り直せば通る）
	clock.advance(500 * time.Millisecond)
	if ok, _ := l.allow(rateLimitRead, "u1"); !ok {
		t.Fatal("0.5 秒待った後の1回が通らない")
	}
	// 叩き続けても、10 秒で戻る 20 回までしか通らない
	clock.advance(10 * time.Second)
	if got := use(l, rateLimitRead, "u1", 100); got != 20 {
		t.Fatalf("10 秒後に通った数 = %d, want 20", got)
	}

	// 利用者が違えば別に数える
	if got := use(l, rateLimitRead, "u2", 120); got != 120 {
		t.Fatalf("別の利用者で通った数 = %d, want 120", got)
	}
	// 読み取りを使い切っても、書き込みは別に数える。書き込みは 30 回、その後は 2 秒に1回
	if got := use(l, rateLimitWrite, "u1", 40); got != 30 {
		t.Fatalf("書き込みで通った数 = %d, want 30", got)
	}
	if _, wait := l.allow(rateLimitWrite, "u1"); wait != 2*time.Second {
		t.Fatalf("書き込みの待ち時間 = %v, want 2s", wait)
	}

	// 時計が戻っても札は増えない
	clock.advance(-time.Hour)
	if ok, _ := l.allow(rateLimitWrite, "u1"); ok {
		t.Fatal("時計が戻ったら札が増えた")
	}
}

func TestUserRateLimiterSweep(t *testing.T) {
	clock := newFakeClock()
	l := newUserRateLimiter(clock.now)
	use(l, rateLimitRead, "idle", 120)
	clock.advance(30 * time.Second)
	use(l, rateLimitWrite, "busy", 30)

	// 前の掃除から1分を過ぎたので、次の allow で掃除が走る。idle の読み取りは 61 秒で満タン（120）に
	// 戻っているので捨てる。busy の書き込みは 31 秒で 15.5 しか戻っていないので残す
	clock.advance(31 * time.Second)
	use(l, rateLimitRead, "other", 1)
	if _, ok := l.buckets["read:idle"]; ok {
		t.Error("満タンに戻った札が残っている")
	}
	if b, ok := l.buckets["write:busy"]; !ok || b.tokens != 15.5 {
		t.Errorf("満タンでない札 = %+v, %v（残して 15.5 のはず）", b, ok)
	}
	if len(l.buckets) != 2 {
		t.Errorf("札の数 = %d, want 2（busy と other）", len(l.buckets))
	}
}

// limitedTestRouter は時計を差し替えた newTestRouter。
func limitedTestRouter() (*router, *fakeClock) {
	rt := newTestRouter()
	clock := newFakeClock()
	rt.rateLimiter = newUserRateLimiter(clock.now)
	return rt, clock
}

func serveAs(rt *router, method, path, as, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	if as != "" {
		req.AddCookie(&http.Cookie{Name: "test", Value: as})
	}
	if ip != "" {
		req.Header.Set("X-Forwarded-For", ip)
	}
	res := httptest.NewRecorder()
	rt.ServeHTTP(res, req)
	return res
}

func TestRouterRateLimit(t *testing.T) {
	rt, clock := limitedTestRouter()

	for i := range 30 {
		if res := serveAs(rt, http.MethodPost, "/api/mine", "alice", ""); res.Code != http.StatusOK {
			t.Fatalf("%d 回目の書き込み: status = %d", i+1, res.Code)
		}
	}
	res := serveAs(rt, http.MethodPost, "/api/mine", "alice", "")
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("31 回目の書き込み: status = %d, want 429", res.Code)
	}
	if got := res.Header().Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After = %q, want 2", got)
	}
	var body apischema.ServerError
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != codeTooManyRequests || body.Error != tooManyRequestsMessage {
		t.Errorf("本文 = %+v", body)
	}

	// 書き込みが尽きても読み取りは通る。管理者のルートも同じ利用者の札で数える
	if res := serveAs(rt, http.MethodGet, "/api/mine", "alice", ""); res.Code != http.StatusOK {
		t.Errorf("読み取り: status = %d", res.Code)
	}
	// ほかの利用者は止まらない
	if res := serveAs(rt, http.MethodPost, "/api/admin/thing", "admin", ""); res.Code != http.StatusOK {
		t.Errorf("別の利用者: status = %d", res.Code)
	}
	// 待てば戻る
	clock.advance(2 * time.Second)
	if res := serveAs(rt, http.MethodPost, "/api/mine", "alice", ""); res.Code != http.StatusOK {
		t.Errorf("2 秒後: status = %d", res.Code)
	}
}

func TestRouterRateLimitDemoPerIP(t *testing.T) {
	rt, _ := limitedTestRouter()

	// デモアカウントは利用者と IP の組で数える。1人が使い切っても、別の IP の面接官は止まらない
	for range 120 {
		serveAs(rt, http.MethodGet, "/api/mine", "demo", "203.0.113.1")
	}
	if res := serveAs(rt, http.MethodGet, "/api/mine", "demo", "203.0.113.1"); res.Code != http.StatusTooManyRequests {
		t.Fatalf("使い切った IP: status = %d, want 429", res.Code)
	}
	if res := serveAs(rt, http.MethodGet, "/api/mine", "demo", "203.0.113.2"); res.Code != http.StatusOK {
		t.Fatalf("別の IP: status = %d, want 200", res.Code)
	}

	// デモ以外は IP を変えても同じ札（IP を変えて回数制限を逃れられない）
	for range 120 {
		serveAs(rt, http.MethodGet, "/api/mine", "alice", "203.0.113.1")
	}
	if res := serveAs(rt, http.MethodGet, "/api/mine", "alice", "203.0.113.9"); res.Code != http.StatusTooManyRequests {
		t.Fatalf("IP を変えた利用者: status = %d, want 429", res.Code)
	}
}

func TestRegisteredRoutesAreRateLimited(t *testing.T) {
	// 本番と同じルートの一覧で、ログインして呼ぶ入口（user・admin）がすべて回数制限を通ることを確かめる。
	// 札を先に使い切っておき、どのルートもハンドラまで来ずに 429 になるかを見る。
	// ログインの入口（auth）は auth_throttle.go が IP とアカウントで別に数えるので対象外。
	rt := newRouter(fakeSessions(testSessions))
	registerAuthRoutes(rt, newAuthHandlers(nil, authConfig{}))
	registerRoutes(rt, nil, jobConfig{simulationEnabled: true}, lineConfig{}, microcmsWebhookConfig{})
	clock := newFakeClock()
	rt.rateLimiter = newUserRateLimiter(clock.now)
	for _, as := range []string{"alice", "admin"} {
		s := testSessions[as]
		use(rt.rateLimiter, rateLimitRead, s.UserID, 120)
		use(rt.rateLimiter, rateLimitWrite, s.UserID, 30)
	}

	checked := 0
	for _, route := range rt.routes {
		as := ""
		switch route.Access {
		case accessUser:
			as = "alice"
		case accessAdmin:
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
				res = serveAs(rt, method, pathParam.ReplaceAllString(path, "1"), as, "")
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
