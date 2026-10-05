package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// LINE 連携のテスト。DB と LINE の API は偽物にする（Go の CI には DB が無い）。
// SQL そのものは line_db_test.go（dbtest タグ）で本物の DB に流して確かめる。

const testChannelSecret = "channel-secret"

// fakeLineStore は lineStore を map で持つ偽物。
type fakeLineStore struct {
	mu          sync.Mutex
	connections map[string]string // userId → lineUserId
	nonces      map[string]fakeNonce
	attempts    map[string]oauthAttempt // state → 試行
	events      map[string]bool         // 処理済みの webhookEventId
	disconnects []string
}

type fakeNonce struct {
	userID  string
	expired bool
}

func newFakeLineStore() *fakeLineStore {
	return &fakeLineStore{
		connections: map[string]string{},
		nonces:      map[string]fakeNonce{},
		attempts:    map[string]oauthAttempt{},
		events:      map[string]bool{},
	}
}

func (f *fakeLineStore) isConnected(_ context.Context, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.connections[userID]
	return ok, nil
}

func (f *fakeLineStore) isLineUserConnected(_ context.Context, lineUserID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ownerOf(lineUserID) != "", nil
}

func (f *fakeLineStore) ownerOf(lineUserID string) string {
	for u, l := range f.connections {
		if l == lineUserID {
			return u
		}
	}
	return ""
}

func (f *fakeLineStore) issueLinkNonce(_ context.Context, userID, nonce string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for n, v := range f.nonces {
		if v.userID == userID {
			delete(f.nonces, n)
		}
	}
	f.nonces[nonce] = fakeNonce{userID: userID}
	return nil
}

func (f *fakeLineStore) completeAccountLink(_ context.Context, nonce, lineUserID string) (accountLinkResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nonces[nonce]
	if !ok || n.expired {
		return accountLinkExpired, nil
	}
	delete(f.nonces, nonce)
	if owner := f.ownerOf(lineUserID); owner != "" && owner != n.userID {
		return accountLinkTaken, nil
	}
	f.connections[n.userID] = lineUserID
	return accountLinkLinked, nil
}

func (f *fakeLineStore) disconnect(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.connections, userID)
	f.disconnects = append(f.disconnects, userID)
	return nil
}

func (f *fakeLineStore) startOAuthAttempt(_ context.Context, state string, a oauthAttempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for s, v := range f.attempts {
		if v.UserID == a.UserID {
			delete(f.attempts, s)
		}
	}
	f.attempts[state] = a
	return nil
}

func (f *fakeLineStore) findOAuthAttempt(_ context.Context, state string) (*oauthAttempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.attempts[state]
	if !ok {
		return nil, nil
	}
	return &a, nil
}

func (f *fakeLineStore) discardOAuthAttempt(_ context.Context, state string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.attempts, state)
	return nil
}

func (f *fakeLineStore) linkVerifiedLineUser(_ context.Context, userID, lineUserID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if owner := f.ownerOf(lineUserID); owner != "" && owner != userID {
		return false, nil
	}
	f.connections[userID] = lineUserID
	return true, nil
}

func (f *fakeLineStore) markWebhookEvent(_ context.Context, eventID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.events[eventID] {
		return true, nil
	}
	f.events[eventID] = true
	return false, nil
}

func (f *fakeLineStore) unmarkWebhookEvent(_ context.Context, eventID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.events, eventID)
	return nil
}

// fakeLineClient は LINE の API の偽物。送ったものを記録する。
type fakeLineClient struct {
	mu       sync.Mutex
	replies  []string // 返信した本文
	pushes   []string
	linkErr  error
	pushErr  error
	friend   bool
	identity lineIdentity
	// exchanged は exchangeCode に渡された値（code・codeVerifier・redirectURI）
	exchanged []string
}

func (f *fakeLineClient) issueLinkToken(_ context.Context, lineUserID string) (string, error) {
	if f.linkErr != nil {
		return "", f.linkErr
	}
	return "link-" + lineUserID, nil
}

func (f *fakeLineClient) replyText(_ context.Context, _, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = append(f.replies, text)
	return nil
}

func (f *fakeLineClient) pushText(_ context.Context, _, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushes = append(f.pushes, text)
	return f.pushErr
}

func (f *fakeLineClient) authCodeURL(state, nonce, _, redirectURI string) (string, error) {
	return "https://access.line.me/authorize?" + url.Values{
		"state": {state}, "nonce": {nonce}, "redirect_uri": {redirectURI},
	}.Encode(), nil
}

func (f *fakeLineClient) exchangeCode(_ context.Context, code, codeVerifier, redirectURI string) (lineTokens, error) {
	f.exchanged = []string{code, codeVerifier, redirectURI}
	return lineTokens{AccessToken: "access", IDToken: "id-token"}, nil
}

func (f *fakeLineClient) verifyIDToken(context.Context, string, string) (lineIdentity, error) {
	return f.identity, nil
}

func (f *fakeLineClient) isFriend(context.Context, string) (bool, error) {
	return f.friend, nil
}

type lineTestEnv struct {
	store  *fakeLineStore
	client *fakeLineClient
	rt     *router
}

func newLineTestEnv() *lineTestEnv {
	env := &lineTestEnv{store: newFakeLineStore(), client: &fakeLineClient{friend: true}}
	h := &lineHandlers{store: env.store, line: env.client, channelSecret: testChannelSecret, webOrigin: "https://juken-map.com"}
	env.rt = newRouter(fakeSessions(testSessions))
	env.rt.user("GET /api/line/connection", h.connection)
	env.rt.user("DELETE /api/line/connection", h.disconnect)
	env.rt.user("POST /api/line/account-link", h.accountLink)
	env.rt.oauth("GET /api/line/oauth/start", h.oauthStart)
	env.rt.oauth("GET /api/line/oauth/callback", h.oauthCallback)
	env.rt.webhook("POST /api/line/webhook", h.webhook)
	env.rt.publicWithSession("GET /line/settings", h.settings)
	return env
}

func (env *lineTestEnv) do(method, target, as, body string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if as != "" {
		req.AddCookie(&http.Cookie{Name: "test", Value: as})
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	env.rt.ServeHTTP(rec, req)
	return rec
}

func lineSign(body string) string {
	mac := hmac.New(sha256.New, []byte(testChannelSecret))
	mac.Write([]byte(body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (env *lineTestEnv) webhook(body string) *httptest.ResponseRecorder {
	return env.do("POST", "/api/line/webhook", "", body, map[string]string{"x-line-signature": lineSign(body)})
}

func TestLineWebhookSignature(t *testing.T) {
	body := `{"events":[{"type":"follow","webhookEventId":"E1","replyToken":"r","source":{"userId":"U1"}}]}`
	tests := []struct {
		name      string
		signature string
		body      string
	}{
		{"署名なし", "", body},
		{"不正な署名", "not-a-signature", body},
		{"別の秘密で作った署名", func() string {
			mac := hmac.New(sha256.New, []byte("other-secret"))
			mac.Write([]byte(body))
			return base64.StdEncoding.EncodeToString(mac.Sum(nil))
		}(), body},
		// 署名は元の本文で作り、本文を1文字だけ変えて送る
		{"本文を1文字変えたもの", lineSign(body), strings.Replace(body, "U1", "U2", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLineTestEnv()
			rec := env.do("POST", "/api/line/webhook", "", tt.body, map[string]string{"x-line-signature": tt.signature})
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			assertJSONEqual(t, rec.Body.String(), `{"error":"Invalid signature"}`)
			if len(env.client.replies) != 0 || len(env.store.events) != 0 {
				t.Errorf("署名が合わないのに処理した: replies=%v events=%v", env.client.replies, env.store.events)
			}
		})
	}

	t.Run("秘密が未設定なら正しい形の署名でも断る", func(t *testing.T) {
		if verifyLineSignature([]byte(body), lineSign(body), "") {
			t.Fatal("秘密が空なのに通った")
		}
	})
}

func TestLineWebhookTooLarge(t *testing.T) {
	env := newLineTestEnv()
	body := `{"events":[],"pad":"` + strings.Repeat("a", lineWebhookBodyLimit) + `"}`
	rec := env.webhook(body)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestLineWebhookRedelivery(t *testing.T) {
	env := newLineTestEnv()
	body := `{"events":[{"type":"message","webhookEventId":"E1","replyToken":"r","source":{"userId":"U1"},"message":{"type":"text","text":"連携"}}]}`

	for i := range 2 {
		if rec := env.webhook(body); rec.Code != http.StatusOK {
			t.Fatalf("%d回目: status = %d", i+1, rec.Code)
		}
	}
	// 同じ webhookEventId の2回目（LINE の再送）は何もしない
	if len(env.client.replies) != 1 {
		t.Fatalf("replies = %v, want 1件", env.client.replies)
	}

	// 別のイベントなら処理する
	env.webhook(strings.Replace(body, "E1", "E2", 1))
	if len(env.client.replies) != 2 {
		t.Fatalf("replies = %v, want 2件", env.client.replies)
	}
}

func TestLineWebhookFailedEventCanBeRedelivered(t *testing.T) {
	env := newLineTestEnv()
	env.client.linkErr = errors.New("LINE API 500")
	body := `{"events":[{"type":"follow","webhookEventId":"E1","replyToken":"r","source":{"userId":"U1"}}]}`

	// イベントの失敗はログに残して 200（Node と同じ）。印は消えて、再送されたらやり直せる。
	if rec := env.webhook(body); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if env.store.events["E1"] {
		t.Fatal("失敗したイベントの印が残っている")
	}
	env.client.linkErr = nil
	env.webhook(body)
	if len(env.client.replies) != 1 || !strings.Contains(env.client.replies[0], "link-U1") {
		t.Fatalf("replies = %v", env.client.replies)
	}
}

func TestLineWebhookEvents(t *testing.T) {
	t.Run("「連携」に公式の Account Link の URL を返す", func(t *testing.T) {
		env := newLineTestEnv()
		env.webhook(`{"events":[{"type":"message","webhookEventId":"E1","replyToken":"r","source":{"userId":"U1"},"message":{"type":"text","text":" 連携 "}}]}`)
		want := "受験マップとLINEを連携します。次のリンクを10分以内に開いてログインしてください。\nhttps://juken-map.com/line/link?linkToken=link-U1"
		if len(env.client.replies) != 1 || env.client.replies[0] != want {
			t.Fatalf("replies = %q", env.client.replies)
		}
	})

	t.Run("連携済みならリンクを出さず通知設定を案内する", func(t *testing.T) {
		env := newLineTestEnv()
		env.store.connections["u1"] = "U1"
		env.webhook(`{"events":[{"type":"follow","webhookEventId":"E1","replyToken":"r","source":{"userId":"U1"}}]}`)
		if len(env.client.replies) != 1 || !strings.HasPrefix(env.client.replies[0], "受験マップとはすでに連携済みです。") {
			t.Fatalf("replies = %q", env.client.replies)
		}
	})

	t.Run("ほかのメッセージには使い方を返す", func(t *testing.T) {
		env := newLineTestEnv()
		env.webhook(`{"events":[{"type":"message","webhookEventId":"E1","replyToken":"r","source":{"userId":"U1"},"message":{"type":"sticker"}}]}`)
		if len(env.client.replies) != 1 || env.client.replies[0] != "受験マップとつなぐには「連携」と送ってください。" {
			t.Fatalf("replies = %q", env.client.replies)
		}
	})

	accountLink := `{"events":[{"type":"accountLink","webhookEventId":"E1","replyToken":"r","source":{"userId":"U1"},"link":{"result":"ok","nonce":"N1"}}]}`

	t.Run("有効な nonce で連携し、nonce を使い捨てにする", func(t *testing.T) {
		env := newLineTestEnv()
		env.store.nonces["N1"] = fakeNonce{userID: "u1"}
		env.webhook(accountLink)
		if env.store.connections["u1"] != "U1" {
			t.Fatalf("connections = %v", env.store.connections)
		}
		if _, ok := env.store.nonces["N1"]; ok {
			t.Fatal("nonce が残っている")
		}
		if !strings.HasPrefix(env.client.replies[0], "受験マップとの連携が完了しました。") {
			t.Fatalf("replies = %q", env.client.replies)
		}
	})

	t.Run("期限切れの nonce なら連携せず、やり直しを案内する", func(t *testing.T) {
		env := newLineTestEnv()
		env.store.nonces["N1"] = fakeNonce{userID: "u1", expired: true}
		env.webhook(accountLink)
		if len(env.store.connections) != 0 {
			t.Fatalf("connections = %v", env.store.connections)
		}
		if !strings.HasPrefix(env.client.replies[0], "連携リンクの期限が切れました。") {
			t.Fatalf("replies = %q", env.client.replies)
		}
	})

	t.Run("別のアカウントに連携済みの LINE なら解除方法を返す", func(t *testing.T) {
		env := newLineTestEnv()
		env.store.connections["other"] = "U1"
		env.store.nonces["N1"] = fakeNonce{userID: "u1"}
		env.webhook(accountLink)
		if env.store.connections["other"] != "U1" || env.store.connections["u1"] != "" {
			t.Fatalf("connections = %v", env.store.connections)
		}
		if !strings.HasPrefix(env.client.replies[0], "このLINEは別の受験マップアカウントに連携済みです。") {
			t.Fatalf("replies = %q", env.client.replies)
		}
	})

	t.Run("連携が失敗の結果なら何もしない", func(t *testing.T) {
		env := newLineTestEnv()
		env.store.nonces["N1"] = fakeNonce{userID: "u1"}
		env.webhook(strings.Replace(accountLink, `"result":"ok"`, `"result":"failed"`, 1))
		if len(env.client.replies) != 0 || len(env.store.connections) != 0 {
			t.Fatalf("replies = %q connections = %v", env.client.replies, env.store.connections)
		}
	})
}

func TestLineConnection(t *testing.T) {
	env := newLineTestEnv()
	env.store.connections["u1"] = "U1"

	rec := env.do("GET", "/api/line/connection", "alice", "", nil)
	assertJSONEqual(t, rec.Body.String(), `{"connected":true}`)

	// デモは解除できない（書き込み）
	if rec := env.do("DELETE", "/api/line/connection", "demo", "", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("demo DELETE status = %d", rec.Code)
	}
	if rec := env.do("DELETE", "/api/line/connection", "", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("未ログインの DELETE status = %d", rec.Code)
	}
	if len(env.store.disconnects) != 0 {
		t.Fatalf("断ったのに解除した: %v", env.store.disconnects)
	}

	rec = env.do("DELETE", "/api/line/connection", "alice", "", nil)
	assertJSONEqual(t, rec.Body.String(), `{"connected":false}`)
	rec = env.do("GET", "/api/line/connection", "alice", "", nil)
	assertJSONEqual(t, rec.Body.String(), `{"connected":false}`)
}

func TestLineAccountLink(t *testing.T) {
	for _, body := range []string{``, `{}`, `{"linkToken":""}`, `{"linkToken":1}`, `{"linkToken":"` + strings.Repeat("a", 256) + `"}`} {
		env := newLineTestEnv()
		rec := env.do("POST", "/api/line/account-link", "alice", body, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, rec.Code)
		}
		assertJSONEqual(t, rec.Body.String(), `{"error":"連携情報が正しくありません"}`)
	}

	env := newLineTestEnv()
	env.store.nonces["old"] = fakeNonce{userID: "u1"}
	rec := env.do("POST", "/api/line/account-link", "alice", `{"linkToken":"T1"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	redirect, _ := url.Parse(decodeJSON(t, rec.Body.String())["redirectUrl"].(string))
	if redirect.Host != "access.line.me" || redirect.Path != "/dialog/bot/accountLink" || redirect.Query().Get("linkToken") != "T1" {
		t.Fatalf("redirectUrl = %s", redirect)
	}
	nonce := redirect.Query().Get("nonce")
	// 古い nonce は捨て、新しい nonce をこの利用者に1つだけ持たせる
	if len(env.store.nonces) != 1 || env.store.nonces[nonce].userID != "u1" {
		t.Fatalf("nonces = %v", env.store.nonces)
	}
}

func TestLineSettings(t *testing.T) {
	env := newLineTestEnv()

	// ログイン済みならプロフィールの通知設定へ
	assertRedirect(t, env.do("GET", "/line/settings", "alice", "", nil), "https://juken-map.com/profile#notification-settings")
	// 未ログインなら通知設定を戻り先にしてログインへ（Node の URLSearchParams と同じく / と # も符号にする）
	assertRedirect(t, env.do("GET", "/line/settings", "", "", nil), "https://juken-map.com/login?callbackURL=%2Fprofile%23notification-settings")
	// セッションを読めないときは 500（ログインへ送って入り直させない）
	if rec := env.do("GET", "/line/settings", "db-down", "", nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("db-down: status = %d, want 500", rec.Code)
	}
}

func TestLineOAuthStart(t *testing.T) {
	env := newLineTestEnv()

	rec := env.do("GET", "/api/line/oauth/start", "", "", nil)
	assertRedirect(t, rec, "https://juken-map.com/login?callbackURL=%2Fprofile%23line-connection")

	// デモと停止中は連携させない
	for _, as := range []string{"demo", "banned"} {
		if rec := env.do("GET", "/api/line/oauth/start", as, "", nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", as, rec.Code)
		}
	}

	env.store.attempts["old"] = oauthAttempt{UserID: "u1"}
	rec = env.do("GET", "/api/line/oauth/start", "alice", "", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	state := loc.Query().Get("state")
	a, ok := env.store.attempts[state]
	if len(env.store.attempts) != 1 || !ok {
		t.Fatalf("attempts = %v", env.store.attempts)
	}
	// 戻り先は固定（リクエストの Host などから組み立てない）
	if a.UserID != "u1" || a.RedirectURI != "https://juken-map.com/api/line/oauth/callback" || a.Nonce != loc.Query().Get("nonce") {
		t.Fatalf("attempt = %+v", a)
	}
}

func TestLineOAuthCallbackState(t *testing.T) {
	callback := "/api/line/oauth/callback?state=S1&code=C1"
	attempt := oauthAttempt{UserID: "u1", Nonce: "N1", CodeVerifier: "V1", RedirectURI: "https://juken-map.com/api/line/oauth/callback"}

	tests := []struct {
		name     string
		target   string
		attempt  *oauthAttempt
		want     string
		keepsRow bool // state が残るべきか
	}{
		{"state なし", "/api/line/oauth/callback?code=C1", &attempt, "invalid", true},
		{"code なし", "/api/line/oauth/callback?state=S1", &attempt, "invalid", true},
		{"state が2つ", "/api/line/oauth/callback?state=S1&state=S2&code=C1", &attempt, "invalid", true},
		{"同意画面でキャンセル", "/api/line/oauth/callback?error=access_denied&state=S1", &attempt, "cancelled", true},
		{"知らない state", callback, nil, "expired", false},
		{"別の人の state（使えず、捨てる）", callback, &oauthAttempt{UserID: "u9", Nonce: "N1"}, "expired", false},
		{"期限切れの state", callback, &oauthAttempt{UserID: "u1", Nonce: "N1", Expired: true}, "expired", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLineTestEnv()
			if tt.attempt != nil {
				env.store.attempts["S1"] = *tt.attempt
			}
			rec := env.do("GET", tt.target, "alice", "", nil)
			assertRedirect(t, rec, "https://juken-map.com/profile?line="+tt.want+"#line-connection")
			if _, ok := env.store.attempts["S1"]; ok != (tt.keepsRow && tt.attempt != nil) {
				t.Errorf("state が残っているか = %v", ok)
			}
			if env.client.exchanged != nil {
				t.Error("使えない state なのに LINE へトークンを取りに行った")
			}
		})
	}

	t.Run("同じ state の2回目（再利用）は使えない", func(t *testing.T) {
		env := newLineTestEnv()
		env.client.identity = lineIdentity{Sub: "U1", Nonce: "N1"}
		env.store.attempts["S1"] = attempt
		assertRedirect(t, env.do("GET", callback, "alice", "", nil), "https://juken-map.com/profile?line=connected#line-connection")
		env.client.exchanged = nil
		assertRedirect(t, env.do("GET", callback, "alice", "", nil), "https://juken-map.com/profile?line=expired#line-connection")
		if env.client.exchanged != nil {
			t.Error("2回目で LINE へトークンを取りに行った")
		}
	})

	t.Run("セッション切れなら戻り先つきでログインへ送り、state は残す", func(t *testing.T) {
		env := newLineTestEnv()
		env.store.attempts["S1"] = attempt
		rec := env.do("GET", callback, "", "", nil)
		assertRedirect(t, rec, "https://juken-map.com/login?callbackURL="+url.QueryEscape("/api/line/oauth/callback?code=C1&state=S1"))
		if _, ok := env.store.attempts["S1"]; !ok {
			t.Error("state が消えた")
		}
	})
}

func TestLineOAuthCallbackLink(t *testing.T) {
	callback := "/api/line/oauth/callback?state=S1&code=C1"
	attempt := oauthAttempt{UserID: "u1", Nonce: "N1", CodeVerifier: "V1", RedirectURI: "https://juken-map.com/api/line/oauth/callback"}

	tests := []struct {
		name     string
		identity lineIdentity
		friend   bool
		pushErr  error
		owner    string // その LINE に連携済みの利用者
		want     string
		linked   bool
	}{
		{"友だちで ID トークンが正しければ連携する", lineIdentity{Sub: "U1", Nonce: "N1"}, true, nil, "", "connected", true},
		{"確認メッセージを送れなくても成功扱い", lineIdentity{Sub: "U1", Nonce: "N1"}, true, errors.New("LINE API 500"), "", "connected", true},
		{"nonce が違えば連携しない", lineIdentity{Sub: "U1", Nonce: "other"}, true, nil, "", "invalid", false},
		{"友だちでなければ連携しない", lineIdentity{Sub: "U1", Nonce: "N1"}, false, nil, "", "friend-required", false},
		{"その LINE が別の人に連携済みなら連携しない", lineIdentity{Sub: "U1", Nonce: "N1"}, true, nil, "other", "already-used", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLineTestEnv()
			env.client.identity, env.client.friend, env.client.pushErr = tt.identity, tt.friend, tt.pushErr
			if tt.owner != "" {
				env.store.connections[tt.owner] = "U1"
			}
			env.store.attempts["S1"] = attempt
			assertRedirect(t, env.do("GET", callback, "alice", "", nil), "https://juken-map.com/profile?line="+tt.want+"#line-connection")

			// 始めたときに残した codeVerifier と戻り先でトークンを取る（PKCE）
			if strings.Join(env.client.exchanged, ",") != "C1,V1,https://juken-map.com/api/line/oauth/callback" {
				t.Errorf("exchanged = %v", env.client.exchanged)
			}
			if (env.store.connections["u1"] == "U1") != tt.linked {
				t.Errorf("connections = %v", env.store.connections)
			}
			if tt.linked && (len(env.client.pushes) != 1 || env.client.pushes[0] != lineConnectionCompletedMessage) {
				t.Errorf("pushes = %q", env.client.pushes)
			}
			if _, ok := env.store.attempts["S1"]; ok {
				t.Error("state が残っている")
			}
		})
	}
}

func assertRedirect(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302（本文 %s）", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != want {
		t.Errorf("Location = %s\nwant       %s", got, want)
	}
}

// TestHTTPLineClientLogin は LINE Login の往復を偽の LINE サーバーに向けて確かめる。
func TestHTTPLineClientLogin(t *testing.T) {
	var tokenForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/v2.1/token":
			raw, _ := io.ReadAll(r.Body)
			tokenForm, _ = url.ParseQuery(string(raw))
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"access_token":"AT","token_type":"Bearer","expires_in":2592000,"id_token":"IDT"}`)
		case "/oauth2/v2.1/verify":
			r.ParseForm()
			if r.Form.Get("id_token") != "IDT" || r.Form.Get("client_id") != "CID" || r.Form.Get("nonce") != "N1" {
				http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
				return
			}
			io.WriteString(w, `{"sub":"U1","nonce":"N1"}`)
		case "/friendship/v1/status":
			if r.Header.Get("Authorization") != "Bearer AT" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			io.WriteString(w, `{"friendFlag":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &httpLineClient{client: srv.Client(), loginBase: srv.URL, authorizeURL: "https://access.line.me/oauth2/v2.1/authorize",
		loginChannelID: "CID", loginSecret: "SECRET"}
	ctx := context.Background()
	redirectURI := "https://juken-map.com/api/line/oauth/callback"

	raw, err := c.authCodeURL("S1", "N1", "VERIFIER", redirectURI)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	sum := sha256.Sum256([]byte("VERIFIER"))
	for k, want := range map[string]string{
		"response_type": "code", "client_id": "CID", "redirect_uri": redirectURI, "state": "S1",
		"scope": "openid profile", "nonce": "N1", "bot_prompt": "aggressive",
		"code_challenge": base64.RawURLEncoding.EncodeToString(sum[:]), "code_challenge_method": "S256",
	} {
		if q.Get(k) != want {
			t.Errorf("認可 URL の %s = %q, want %q", k, q.Get(k), want)
		}
	}

	tokens, err := c.exchangeCode(ctx, "C1", "VERIFIER", redirectURI)
	if err != nil {
		t.Fatal(err)
	}
	if tokens != (lineTokens{AccessToken: "AT", IDToken: "IDT"}) {
		t.Errorf("tokens = %+v", tokens)
	}
	// LINE は client_secret を本文で受け取る。PKCE の code_verifier も本文で送る。
	for k, want := range map[string]string{
		"grant_type": "authorization_code", "code": "C1", "redirect_uri": redirectURI,
		"client_id": "CID", "client_secret": "SECRET", "code_verifier": "VERIFIER",
	} {
		if tokenForm.Get(k) != want {
			t.Errorf("トークン要求の %s = %q, want %q", k, tokenForm.Get(k), want)
		}
	}

	identity, err := c.verifyIDToken(ctx, "IDT", "N1")
	if err != nil || identity != (lineIdentity{Sub: "U1", Nonce: "N1"}) {
		t.Errorf("identity = %+v, err = %v", identity, err)
	}
	if friend, err := c.isFriend(ctx, "AT"); err != nil || !friend {
		t.Errorf("friend = %v, err = %v", friend, err)
	}
	if _, err := c.verifyIDToken(ctx, "IDT", "wrong"); err == nil {
		t.Error("LINE が 400 を返したのにエラーにならない")
	}

	// 設定が無ければ LINE を呼ばずにエラー
	if _, err := (&httpLineClient{}).authCodeURL("S1", "N1", "V", redirectURI); err == nil {
		t.Error("LINE_LOGIN_CHANNEL_ID が空なのにエラーにならない")
	}
}
