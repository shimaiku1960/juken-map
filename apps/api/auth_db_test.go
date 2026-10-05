//go:build dbtest

// ログインの流れを本物の MySQL に流すテスト（JUK-115）。テスト名の B2・C3 などは認証基準 10 の項目、
// 06 で始まるものは 06_security.md の項目で、それぞれの「測り方」にあたる。
//
// 時刻はテストの時計（authEnv.clock）で進める。メールは送らずに受け箱（fakeMailbox）へ入れる。
// 利用者・回数制限・メールの記録は、テストごとに作った値だけを最後に消す。
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const authTestPassword = "a long passphrase for tests"

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type sentMail struct{ to, subject, html string }

type fakeMailbox struct {
	mu   sync.Mutex
	sent []sentMail
}

func (m *fakeMailbox) send(_ context.Context, to, subject, html string) (http.Header, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{to, subject, html})
	return http.Header{}, nil
}

// last は to 宛ての最後のメール。件名に subject を含むものに絞る。
func (m *fakeMailbox) last(t *testing.T, to, subject string) sentMail {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if m.sent[i].to == to && strings.Contains(m.sent[i].subject, subject) {
			return m.sent[i]
		}
	}
	t.Fatalf("%s 宛ての「%s」のメールが無い（%d 通）", to, subject, len(m.sent))
	return sentMail{}
}

func (m *fakeMailbox) count(to, subject string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.sent {
		if s.to == to && strings.Contains(s.subject, subject) {
			n++
		}
	}
	return n
}

var tokenInLink = regexp.MustCompile(`token=([A-Za-z0-9_-]{43})`)

func (s sentMail) token(t *testing.T) string {
	t.Helper()
	m := tokenInLink.FindStringSubmatch(s.html)
	if m == nil {
		t.Fatalf("メールにトークンが無い: %s", s.html)
	}
	return m[1]
}

type authEnv struct {
	t      *testing.T
	db     *sql.DB
	fx     dbFixture
	h      *authHandlers
	rt     *router
	clock  *testClock
	mails  *fakeMailbox
	ips    []string
	emails []string
}

func newAuthEnv(t *testing.T) *authEnv {
	db := openTestDB(t)
	keys, err := newTOTPKeyring("", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	e := &authEnv{
		t: t, db: db, fx: dbFixture{t, db},
		clock: &testClock{now: time.Now().UTC().Truncate(time.Millisecond)},
		mails: &fakeMailbox{},
	}
	e.h = newAuthHandlers(db, authConfig{
		webOrigin: "https://juken-map.com", totpKeys: keys, hashConcurrency: 2,
		adminTo: e.newEmail(), sender: e.mails, now: e.clock.Now,
		async: func(f func()) { f() },
	})
	e.rt = newRouter((&sessionAuth{store: e.h.sessions}).load)
	registerAuthRoutes(e.rt, e.h)
	registerRoutes(e.rt, db, jobConfig{}, lineConfig{webOrigin: "https://juken-map.com"}, microcmsWebhookConfig{})
	t.Cleanup(e.cleanup)
	return e
}

// newEmail はテストで使うメールアドレス。最後にその利用者・回数制限・メールの記録を消す。
func (e *authEnv) newEmail() string {
	email := "auth-" + testHex(6) + "@auth-test.example"
	e.emails = append(e.emails, email)
	return email
}

func (e *authEnv) cleanup() {
	for _, email := range e.emails {
		e.fx.exec("DELETE FROM `user` WHERE email = ?", email)
		e.fx.exec("DELETE FROM EmailSend WHERE recipientHash = ?", recipientHash(email))
		for _, rule := range []throttleRule{throttleSignInAccount} {
			e.fx.exec("DELETE FROM AuthThrottle WHERE bucket = ?", throttleBucket(rule, email))
		}
	}
	for _, ip := range e.ips {
		for _, rule := range []throttleRule{throttleSignInIP, throttleAnonymousIP} {
			e.fx.exec("DELETE FROM AuthThrottle WHERE bucket = ?", throttleBucket(rule, ip))
		}
	}
}

// browser は Cookie を覚えるブラウザ。Set-Cookie の MaxAge が負なら消す。ブラウザごとに別の IP から来る
// （IP 単位の回数制限に、テストの組み立ての都合で当たらないように）。
type browser struct {
	env     *authEnv
	ip      string
	cookies map[string]*http.Cookie
}

func (e *authEnv) browser() *browser {
	ip := fmt.Sprintf("10.%d.%d.%d", randomBytes(1)[0], randomBytes(1)[0], randomBytes(1)[0])
	e.ips = append(e.ips, ip)
	return &browser{env: e, ip: ip, cookies: map[string]*http.Cookie{}}
}

func (b *browser) do(method, path string, body any) *httptest.ResponseRecorder {
	b.env.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "https://juken-map.com"+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Forwarded-For", b.ip)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range b.cookies {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	rec := httptest.NewRecorder()
	b.env.rt.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c
		}
	}
	return rec
}

func (b *browser) sessionEmail() string {
	b.env.t.Helper()
	rec := b.do("GET", "/api/auth/session", nil)
	var res *SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		b.env.t.Fatalf("session: %s", rec.Body)
	}
	if res == nil {
		return ""
	}
	return res.User.Email
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d（本文 %s）", rec.Code, status, rec.Body)
	}
	if code != "" && !strings.Contains(rec.Body.String(), `"code":"`+code+`"`) {
		t.Fatalf("code が %s でない: %s", code, rec.Body)
	}
}

// signUpVerified はメール＋パスワードで登録して確認まで済ませた利用者を作る。
func (e *authEnv) signUpVerified(email, password string) string {
	e.t.Helper()
	b := e.browser()
	expectStatus(e.t, b.do("POST", "/api/auth/sign-up", map[string]string{"email": email, "password": password}), 200, "")
	token := e.mails.last(e.t, email, "メールアドレスの確認").token(e.t)
	expectStatus(e.t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": token}), 200, "")
	var id string
	if err := e.db.QueryRow("SELECT id FROM `user` WHERE email = ?", email).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *authEnv) signedIn(email, password string) *browser {
	e.t.Helper()
	b := e.browser()
	expectStatus(e.t, b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": password}), 200, "")
	return b
}

func TestAuthDBSignUpVerifyAndSignIn(t *testing.T) {
	e := newAuthEnv(t)
	email := e.newEmail()
	b := e.browser()

	expectStatus(t, b.do("POST", "/api/auth/sign-up", map[string]string{"email": email, "password": "short"}), 400, "WEAK_PASSWORD")
	expectStatus(t, b.do("POST", "/api/auth/sign-up", map[string]string{"email": strings.ToUpper(email), "password": authTestPassword}), 200, "")

	// 06 B3：確認が済むまでログインできない（パスワードが合っていても）。
	expectStatus(t, b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 403, "EMAIL_NOT_VERIFIED")

	mail := e.mails.last(t, email, "メールアドレスの確認")
	if !strings.Contains(mail.html, "https://juken-map.com/verify-email/confirm?token=") {
		t.Fatalf("リンクが確認の画面を指していない（E2）: %s", mail.html)
	}
	token := mail.token(t)
	// E1：DB にはトークンそのものではなく、そのハッシュが入っている。
	if e.fx.count("SELECT COUNT(*) FROM AuthToken WHERE tokenHash = ?", hashToken(token)) != 1 {
		t.Fatal("トークンのハッシュが保存されていない")
	}
	expectStatus(t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": token}), 200, "")
	e.mails.last(t, e.h.mailer.adminTo, "新しいユーザー")
	// E1：1回使ったトークンはもう使えない。
	expectStatus(t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": token}), 400, "INVALID_TOKEN")

	rec := b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
	expectStatus(t, rec, 200, "")
	// D1：セッションの Cookie の属性。
	var c *http.Cookie
	for _, got := range rec.Result().Cookies() {
		if got.Name == sessionCookieName {
			c = got
		}
	}
	if c == nil || !c.HttpOnly || !c.Secure || c.Path != "/" || c.Domain != "" || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("Set-Cookie = %+v", c)
	}
	if c.MaxAge < int((30*24*time.Hour-time.Minute)/time.Second) || c.MaxAge > int(30*24*time.Hour/time.Second) {
		t.Fatalf("MaxAge = %d（30日に合わせる）", c.MaxAge)
	}
	if got := b.sessionEmail(); got != email {
		t.Fatalf("session = %q", got)
	}

	// C2：DB の値（トークンのハッシュ）で Cookie を作っても入れない。
	var stored []byte
	if err := e.db.QueryRow("SELECT s.tokenHash FROM AuthSession s JOIN `user` u ON u.id = s.userId WHERE u.email = ?", email).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	forged := e.browser()
	forged.cookies[sessionCookieName] = &http.Cookie{Name: sessionCookieName, Value: base64.RawURLEncoding.EncodeToString(stored)}
	if forged.sessionEmail() != "" {
		t.Fatal("DB の値から作った Cookie で入れた")
	}
	expectStatus(t, forged.do("GET", "/api/dashboard", nil), 401, "")
}

func TestAuthDBSignUpDoesNotRevealExistingAccount(t *testing.T) {
	// 06 B4・10 H2：登録済みのメールアドレスでも応答は同じで、アカウントは変わらない。本人へは知らせる。
	e := newAuthEnv(t)
	existing := e.newEmail()
	e.signUpVerified(existing, authTestPassword)
	fresh := e.newEmail()
	b := e.browser()
	a := b.do("POST", "/api/auth/sign-up", map[string]string{"email": existing, "password": "another long passphrase"})
	n := b.do("POST", "/api/auth/sign-up", map[string]string{"email": fresh, "password": "another long passphrase"})
	if a.Code != n.Code || a.Body.String() != n.Body.String() {
		t.Fatalf("応答が違う: %d %s / %d %s", a.Code, a.Body, n.Code, n.Body)
	}
	e.mails.last(t, existing, "登録済み")
	expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": existing, "password": authTestPassword}), 200, "")
	expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": existing, "password": "another long passphrase"}), 401, "INVALID_CREDENTIALS")
}

func TestAuthDBSignUpAgainBeforeVerifying(t *testing.T) {
	// 確認前に登録し直すと、パスワードは新しいものに置き換わり、古い確認のリンクは使えなくなる。
	e := newAuthEnv(t)
	email := e.newEmail()
	b := e.browser()
	expectStatus(t, b.do("POST", "/api/auth/sign-up", map[string]string{"email": email, "password": "the attacker passphrase"}), 200, "")
	first := e.mails.last(t, email, "メールアドレスの確認").token(t)
	expectStatus(t, b.do("POST", "/api/auth/sign-up", map[string]string{"email": email, "password": authTestPassword}), 200, "")
	second := e.mails.last(t, email, "メールアドレスの確認").token(t)
	expectStatus(t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": first}), 400, "INVALID_TOKEN")
	expectStatus(t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": second}), 200, "")
	expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": "the attacker passphrase"}), 401, "")
	expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 200, "")
}

func TestAuthDBSignInFailuresLookTheSame(t *testing.T) {
	// 06 B4：存在しないメールアドレスと、違うパスワードで、応答（状態と本文）が同じ。
	e := newAuthEnv(t)
	email := e.newEmail()
	e.signUpVerified(email, authTestPassword)
	unknown := e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": e.newEmail(), "password": authTestPassword})
	wrong := e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": "not the right passphrase"})
	if unknown.Code != 401 || unknown.Code != wrong.Code || unknown.Body.String() != wrong.Body.String() {
		t.Fatalf("応答が違う: %d %s / %d %s", unknown.Code, unknown.Body, wrong.Code, wrong.Body)
	}
}

func TestAuthDBSignInThrottlePerAccount(t *testing.T) {
	// 06 B4・10 H1：アカウント単位で 10 回まで。窓（15 分）が終わるまでは正しいパスワードでも止まる。
	e := newAuthEnv(t)
	email := e.newEmail()
	e.signUpVerified(email, authTestPassword)
	for i := range throttleSignInAccount.max {
		// ブラウザ（IP）を変えながら試しても数える（IP 単位では止まらない攻撃）。
		expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": "wrong passphrase " + fmt.Sprint(i)}), 401, "")
	}
	rec := e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
	expectStatus(t, rec, 429, "TOO_MANY_SIGN_IN_ATTEMPTS")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After が無い")
	}
	e.clock.Advance(throttleSignInAccount.window + time.Second)
	expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 200, "")
}

func TestAuthDBSessionExpiry(t *testing.T) {
	e := newAuthEnv(t)

	t.Run("C3 一般の利用者は30日で切れ、使っても延びない", func(t *testing.T) {
		email := e.newEmail()
		e.signUpVerified(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		for range 29 {
			e.clock.Advance(24 * time.Hour)
			if b.sessionEmail() != email {
				t.Fatal("30日より前に切れた")
			}
		}
		e.clock.Advance(24*time.Hour + time.Second)
		if b.sessionEmail() != "" {
			t.Fatal("30日を過ぎても使えた")
		}
		expectStatus(t, b.do("GET", "/api/dashboard", nil), 401, "")
	})

	t.Run("C3 管理者は使わないと1時間で切れる", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		e.fx.exec("UPDATE `user` SET role = 'admin' WHERE id = ?", id)
		b := e.signedIn(email, authTestPassword)
		e.clock.Advance(59 * time.Minute)
		if b.sessionEmail() != email {
			t.Fatal("1時間より前に切れた")
		}
		e.clock.Advance(59 * time.Minute)
		if b.sessionEmail() != email {
			t.Fatal("使ったのに切れた")
		}
		e.clock.Advance(61*time.Minute + time.Second)
		if b.sessionEmail() != "" {
			t.Fatal("使わずに1時間を過ぎても使えた")
		}
	})

	t.Run("C3 管理者は使い続けても24時間で切れる", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		e.fx.exec("UPDATE `user` SET role = 'admin' WHERE id = ?", id)
		b := e.signedIn(email, authTestPassword)
		for range 28 {
			e.clock.Advance(50 * time.Minute)
			if b.sessionEmail() != email {
				t.Fatal("24時間より前に切れた")
			}
		}
		e.clock.Advance(50 * time.Minute)
		if b.sessionEmail() != "" {
			t.Fatal("24時間を過ぎても使えた")
		}
	})
}

func TestAuthDBSessionRevocation(t *testing.T) {
	e := newAuthEnv(t)
	email := e.newEmail()
	id := e.signUpVerified(email, authTestPassword)

	t.Run("C4 ログインし直すと前のトークンは使えない", func(t *testing.T) {
		b := e.signedIn(email, authTestPassword)
		before := *b.cookies[sessionCookieName]
		expectStatus(t, b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 200, "")
		if b.cookies[sessionCookieName].Value == before.Value {
			t.Fatal("同じトークンのまま")
		}
		old := e.browser()
		old.cookies[sessionCookieName] = &before
		if old.sessionEmail() != "" {
			t.Fatal("前のトークンがまだ使える")
		}
	})

	t.Run("C5-1 ログアウトでこの端末だけ消える", func(t *testing.T) {
		a := e.signedIn(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		saved := *a.cookies[sessionCookieName]
		expectStatus(t, a.do("POST", "/api/auth/sign-out", map[string]any{}), 200, "")
		a.cookies[sessionCookieName] = &saved
		if a.sessionEmail() != "" || b.sessionEmail() != email {
			t.Fatal("ログアウトした端末だけが消えていない")
		}
	})

	t.Run("C5-2・06 B6・10 E4 パスワード変更でほかの端末が消え、知らせる", func(t *testing.T) {
		a := e.signedIn(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		expectStatus(t, a.do("POST", "/api/auth/password/change", map[string]string{"currentPassword": "wrong current passphrase", "newPassword": "a brand new passphrase"}), 400, "INVALID_PASSWORD")
		expectStatus(t, a.do("POST", "/api/auth/password/change", map[string]string{"currentPassword": authTestPassword, "newPassword": "a brand new passphrase"}), 200, "")
		if a.sessionEmail() != email || b.sessionEmail() != "" {
			t.Fatal("変えた端末だけが残っていない")
		}
		e.mails.last(t, email, "パスワードが変更されました")
		expectStatus(t, a.do("POST", "/api/auth/password/change", map[string]string{"currentPassword": "a brand new passphrase", "newPassword": authTestPassword}), 200, "")
	})

	t.Run("C5-3 管理画面の停止でその人の全端末が消え、次のログインも断る", func(t *testing.T) {
		a := e.signedIn(email, authTestPassword)
		removed, err := (&sqlAdminUserStore{db: e.db}).ban(context.Background(), id, e.clock.Now())
		if err != nil || removed == 0 {
			t.Fatalf("ban = %d, %v", removed, err)
		}
		if a.sessionEmail() != "" {
			t.Fatal("停止した人のセッションが残っている")
		}
		expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 403, "ACCOUNT_BANNED")
		e.fx.exec("UPDATE `user` SET bannedAt = NULL WHERE id = ?", id)
	})
}

// 06 B2：ログアウトした Cookie では API が 401 になる。ブラウザが Cookie を消さなかったとき（盗まれた
// Cookie が残っているとき）も、サーバーの側で無効になっていることを見る。ログインで渡す Cookie の属性は
// TestAuthDBSignUpVerifyAndSignIn（D1）で見ている。
func TestAuthDB06B2SignOutRevokesCookie(t *testing.T) {
	e := newAuthEnv(t)
	email := e.newEmail()
	e.signUpVerified(email, authTestPassword)
	b := e.signedIn(email, authTestPassword)
	saved := *b.cookies[sessionCookieName]
	expectStatus(t, b.do("GET", "/api/dashboard", nil), 200, "")

	rec := b.do("POST", "/api/auth/sign-out", map[string]any{})
	expectStatus(t, rec, 200, "")
	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			cleared = c
		}
	}
	// 消すときも同じ属性で送る（属性が違うと、ブラウザによっては別の Cookie として扱われ、消えない）。
	if cleared == nil || cleared.MaxAge >= 0 || cleared.Value != "" || cleared.Path != "/" ||
		!cleared.HttpOnly || !cleared.Secure || cleared.SameSite != http.SameSiteStrictMode {
		t.Fatalf("ログアウトの Set-Cookie = %+v", cleared)
	}

	b.cookies[sessionCookieName] = &saved
	expectStatus(t, b.do("GET", "/api/dashboard", nil), 401, "")
	expectStatus(t, b.do("POST", "/api/study-logs", map[string]any{}), 401, "")
	if b.sessionEmail() != "" {
		t.Fatal("ログアウトした Cookie でセッションが返った")
	}
}

// 06 B3：メールの確認が済むまで、利用者の API は使えない。パスワードが合っていてもセッションを渡さない。
// 外部ログインで、プロバイダーが確認済みと示さないメールを断るのは TestAuthDBGoogleLogin（F2）で見ている。
func TestAuthDB06B3UnverifiedCannotUseAPI(t *testing.T) {
	e := newAuthEnv(t)
	email := e.newEmail()
	b := e.browser()
	expectStatus(t, b.do("POST", "/api/auth/sign-up", map[string]string{"email": email, "password": authTestPassword}), 200, "")
	expectStatus(t, b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 403, "EMAIL_NOT_VERIFIED")
	if len(b.cookies) != 0 {
		t.Fatalf("未確認なのに Cookie が付いた: %v", b.cookies)
	}
	expectStatus(t, b.do("GET", "/api/dashboard", nil), 401, "")
	expectStatus(t, b.do("POST", "/api/study-logs", map[string]any{}), 401, "")
	expectStatus(t, b.do("POST", "/api/auth/mfa/setup", map[string]any{}), 401, "")

	// 確認を済ませれば同じパスワードで使える（断った理由が確認だけであること）。
	token := e.mails.last(t, email, "メールアドレスの確認").token(t)
	expectStatus(t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": token}), 200, "")
	expectStatus(t, b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 200, "")
	expectStatus(t, b.do("GET", "/api/dashboard", nil), 200, "")
}

func TestAuthDBPasswordReset(t *testing.T) {
	e := newAuthEnv(t)
	email := e.newEmail()
	e.signUpVerified(email, authTestPassword)
	signedIn := e.signedIn(email, authTestPassword)
	b := e.browser()

	// 存在しないメールアドレスでも応答は同じ（06 B4）。
	unknown := b.do("POST", "/api/auth/password/forgot", map[string]string{"email": e.newEmail()})
	known := b.do("POST", "/api/auth/password/forgot", map[string]string{"email": email})
	if unknown.Code != 200 || unknown.Body.String() != known.Body.String() {
		t.Fatalf("応答が違う: %s / %s", unknown.Body, known.Body)
	}
	token := e.mails.last(t, email, "パスワードの再設定").token(t)

	// E1：用途の違うトークン（確認用）では再設定できない。
	verifyToken, err := e.h.issueToken(context.Background(), e.userID(email), tokenPurposeVerifyEmail, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, b.do("POST", "/api/auth/password/reset", map[string]string{"token": verifyToken, "password": "a brand new passphrase"}), 400, "INVALID_TOKEN")
	// 規則に合わないパスワードではトークンを使わない（直して送り直せる）。
	expectStatus(t, b.do("POST", "/api/auth/password/reset", map[string]string{"token": token, "password": "short"}), 400, "WEAK_PASSWORD")

	rec := b.do("POST", "/api/auth/password/reset", map[string]string{"token": token, "password": "a brand new passphrase"})
	expectStatus(t, rec, 200, "")
	// E3：自動でログインさせない。全セッションが消える。
	if _, ok := b.cookies[sessionCookieName]; ok {
		t.Fatal("再設定でログインした")
	}
	if signedIn.sessionEmail() != "" {
		t.Fatal("再設定の前のセッションが残っている")
	}
	// E1：1回だけ。E3：まだ使われていないトークン（確認用）も消える。
	expectStatus(t, b.do("POST", "/api/auth/password/reset", map[string]string{"token": token, "password": "yet another passphrase"}), 400, "INVALID_TOKEN")
	expectStatus(t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": verifyToken}), 400, "INVALID_TOKEN")
	e.mails.last(t, email, "パスワードが変更されました")
	expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": "a brand new passphrase"}), 200, "")

	// E1：寿命は1時間。
	expectStatus(t, b.do("POST", "/api/auth/password/forgot", map[string]string{"email": email}), 200, "")
	late := e.mails.last(t, email, "パスワードの再設定").token(t)
	e.clock.Advance(passwordResetTTL + time.Second)
	expectStatus(t, b.do("POST", "/api/auth/password/reset", map[string]string{"token": late, "password": "yet another passphrase"}), 400, "INVALID_TOKEN")
	// 新しく発行したら古いものは無効（E1）。
	expectStatus(t, b.do("POST", "/api/auth/password/forgot", map[string]string{"email": email}), 200, "")
	first := e.mails.last(t, email, "パスワードの再設定").token(t)
	expectStatus(t, b.do("POST", "/api/auth/password/forgot", map[string]string{"email": email}), 200, "")
	expectStatus(t, b.do("POST", "/api/auth/password/reset", map[string]string{"token": first, "password": "yet another passphrase"}), 400, "INVALID_TOKEN")
}

func (e *authEnv) userID(email string) string {
	e.t.Helper()
	var id string
	if err := e.db.QueryRow("SELECT id FROM `user` WHERE email = ?", email).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestAuthDBLegacyPasswordIsRehashed(t *testing.T) {
	// B2：古い方式（Better Auth の scrypt）のハッシュでログインすると、Argon2id に作り直される。
	e := newAuthEnv(t)
	email := e.newEmail()
	id := e.signUpVerified(email, authTestPassword)
	const legacy = "00112233445566778899aabbccddeeff:c3ed6e7eb77125c0e5bce6a24fb96b9e99e4fdc6e82b2cbdea90bdceb95a421cd78e9bb23c184fdf83e175d3635b65c2673c65efa8eb4e52e4b623b7c36065d8"
	e.fx.exec("UPDATE AuthPassword SET hash = ? WHERE userId = ?", legacy, id)
	expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": "legacy-password-1234"}), 200, "")
	var hash string
	if err := e.db.QueryRow("SELECT hash FROM AuthPassword WHERE userId = ?", id).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("作り直されていない: %s", hash)
	}
	expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": "legacy-password-1234"}), 200, "")
}

// enableMFA は2段階認証を有効にし、TOTP の秘密と予備コードを返す。
func (e *authEnv) enableMFA(b *browser) ([]byte, []string) {
	e.t.Helper()
	rec := b.do("POST", "/api/auth/mfa/setup", map[string]string{"password": authTestPassword})
	expectStatus(e.t, rec, 200, "")
	var res struct {
		TotpURI     string   `json:"totpURI"`
		BackupCodes []string `json:"backupCodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(res.TotpURI)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(u.Query().Get("secret"))
	if err != nil || len(secret) != totpSecretSize {
		e.t.Fatalf("secret: %v (%d)", err, len(secret))
	}
	expectStatus(e.t, b.do("POST", "/api/auth/mfa/confirm", map[string]string{"code": totpCode(secret, totpStep(e.clock.Now()))}), 200, "")
	return secret, res.BackupCodes
}

func TestAuthDBMFA(t *testing.T) {
	e := newAuthEnv(t)
	email := e.newEmail()
	id := e.signUpVerified(email, authTestPassword)
	e.fx.exec("UPDATE `user` SET role = 'admin' WHERE id = ?", id)
	b := e.signedIn(email, authTestPassword)

	// 06 B7：2段階認証を通していない管理者のセッションは、管理 API で 403。
	expectStatus(t, b.do("GET", "/api/admin/overview", nil), 403, twoFactorRequired)
	// E4：設定には今のパスワードの入れ直しが要る。
	expectStatus(t, b.do("POST", "/api/auth/mfa/setup", map[string]string{"password": "not my passphrase!!"}), 400, "INVALID_PASSWORD")
	before := b.cookies[sessionCookieName].Value
	secret, backup := e.enableMFA(b)
	// C4：有効にしたらセッションを作り直す。G3：2段階認証を済ませたセッションなので管理 API が通る。
	if b.cookies[sessionCookieName].Value == before {
		t.Fatal("2段階認証のあとにセッションが作り直されていない")
	}
	expectStatus(t, b.do("GET", "/api/admin/overview", nil), 200, "")
	e.mails.last(t, email, "2段階認証を有効にしました")
	// G2：予備コードは平文でも暗号文でもなく、ハッシュで保存されている。G1：秘密は暗号化されている。
	var sealed string
	if err := e.db.QueryRow("SELECT secret FROM AuthTotp WHERE userId = ?", id).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)) || !strings.HasPrefix(sealed, "v0:") {
		t.Fatalf("秘密が暗号化されていない: %s", sealed)
	}
	if e.fx.count("SELECT COUNT(*) FROM AuthBackupCode WHERE userId = ? AND codeHash = ?", id, hashBackupCode(backup[0])) != 1 {
		t.Fatal("予備コードのハッシュが無い")
	}

	t.Run("G3 パスワードだけではセッションにならない", func(t *testing.T) {
		p := e.browser()
		rec := p.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
		expectStatus(t, rec, 200, "")
		if !strings.Contains(rec.Body.String(), `"mfaRequired":true`) {
			t.Fatalf("本文 %s", rec.Body)
		}
		if _, ok := p.cookies[sessionCookieName]; ok {
			t.Fatal("2段階認証の前にセッションの Cookie が置かれた")
		}
		if c := p.cookies[mfaCookieName]; c == nil || c.SameSite != http.SameSiteLaxMode || c.MaxAge > int(mfaChallengeTTL/time.Second) {
			t.Fatalf("途中の状態の Cookie = %+v", c)
		}
		expectStatus(t, p.do("GET", "/api/dashboard", nil), 401, "")
		expectStatus(t, p.do("GET", "/api/admin/overview", nil), 401, "")
	})

	t.Run("C4 2段階認証でログインし直すと、同じブラウザの前のセッションは使えない", func(t *testing.T) {
		e.clock.Advance(totpPeriod)
		before := *b.cookies[sessionCookieName]
		expectStatus(t, b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 200, "")
		expectStatus(t, b.do("POST", "/api/auth/mfa/verify", map[string]string{"code": totpCode(secret, totpStep(e.clock.Now()))}), 200, "")
		old := e.browser()
		old.cookies[sessionCookieName] = &before
		if old.sessionEmail() != "" || b.sessionEmail() != email {
			t.Fatal("前のセッションが残っているか、新しいセッションで入れない")
		}
	})

	t.Run("G1 コードは1回だけ・途中の状態ごとに5回まで", func(t *testing.T) {
		e.clock.Advance(totpPeriod)
		p := e.browser()
		p.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
		for range mfaChallengeMaxAttempts {
			expectStatus(t, p.do("POST", "/api/auth/mfa/verify", map[string]string{"code": "000000"}), 401, "INVALID_CODE")
		}
		expectStatus(t, p.do("POST", "/api/auth/mfa/verify", map[string]string{"code": totpCode(secret, totpStep(e.clock.Now()))}), 401, "MFA_CHALLENGE_EXPIRED")
		dbFixture{t, e.db}.exec("DELETE FROM AuthThrottle WHERE bucket = ?", throttleBucket(throttleMFAAccount, id))

		p.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
		code := totpCode(secret, totpStep(e.clock.Now()))
		expectStatus(t, p.do("POST", "/api/auth/mfa/verify", map[string]string{"code": code}), 200, "")
		expectStatus(t, p.do("GET", "/api/admin/overview", nil), 200, "")

		q := e.browser()
		q.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
		expectStatus(t, q.do("POST", "/api/auth/mfa/verify", map[string]string{"code": code}), 401, "INVALID_CODE")
	})

	t.Run("G2 予備コードは1回だけ", func(t *testing.T) {
		p := e.browser()
		p.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
		rec := p.do("POST", "/api/auth/mfa/verify", map[string]string{"code": strings.ToUpper(backup[1]), "method": "backup"})
		expectStatus(t, rec, 200, "")
		if !strings.Contains(rec.Body.String(), `"backupCodesRemaining":9`) {
			t.Fatalf("残りの数: %s", rec.Body)
		}
		q := e.browser()
		q.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
		expectStatus(t, q.do("POST", "/api/auth/mfa/verify", map[string]string{"code": backup[1], "method": "backup"}), 401, "INVALID_CODE")
	})

	t.Run("E3 再設定のあとも2段階認証を求める", func(t *testing.T) {
		e.browser().do("POST", "/api/auth/password/forgot", map[string]string{"email": email})
		token := e.mails.last(t, email, "パスワードの再設定").token(t)
		expectStatus(t, e.browser().do("POST", "/api/auth/password/reset", map[string]string{"token": token, "password": authTestPassword + "!"}), 200, "")
		rec := e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword + "!"})
		if !strings.Contains(rec.Body.String(), `"mfaRequired":true`) {
			t.Fatalf("再設定で2段階認証が外れた: %s", rec.Body)
		}
	})
}

// fakeGoogle は Google のトークンエンドポイントの偽物。同意画面で渡した PKCE の challenge と nonce を覚え、
// code_verifier が合うときだけ ID トークンを返す。
type fakeGoogle struct {
	t         *testing.T
	server    *httptest.Server
	mu        sync.Mutex
	challenge string
	nonce     string
	claims    map[string]any
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	g := &fakeGoogle{t: t}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			g.t.Error(err)
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		claims := map[string]any{"iss": "https://accounts.google.com", "aud": "google-client", "exp": time.Now().Add(time.Hour).Unix(), "nonce": g.nonce}
		for k, v := range g.claims {
			claims[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": fakeIDToken(claims)}); err != nil {
			g.t.Error(err)
		}
	}))
	t.Cleanup(g.server.Close)
	return g
}

// start は画面の「Google でログイン」から同意画面の URL を受け取るところまで。state を返す。
func (g *fakeGoogle) start(b *browser, callbackURL string) string {
	g.t.Helper()
	rec := b.do("POST", "/api/auth/oauth/google", map[string]string{"callbackURL": callbackURL})
	expectStatus(g.t, rec, 200, "")
	var res struct{ URL string }
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		g.t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("scope") != "openid email" {
		g.t.Fatalf("同意画面の URL: %s", res.URL)
	}
	g.mu.Lock()
	g.challenge, g.nonce = q.Get("code_challenge"), q.Get("nonce")
	g.mu.Unlock()
	return q.Get("state")
}

func TestAuthDBGoogleLogin(t *testing.T) {
	e := newAuthEnv(t)
	g := newFakeGoogle(t)
	e.h.oauth = newOAuthProviders("https://juken-map.com", oauthEndpoints{googleAuth: g.server.URL + "/auth", googleToken: g.server.URL + "/token"},
		"google-client", "google-secret", "", "")
	callback := func(b *browser, state, code string) *httptest.ResponseRecorder {
		return b.do("GET", "/api/auth/callback/google?code="+code+"&state="+url.QueryEscape(state), nil)
	}

	t.Run("新しい利用者を作ってログインし、戻り先へ送る", func(t *testing.T) {
		email := e.newEmail()
		g.claims = map[string]any{"sub": "g-" + testHex(6), "email": email, "email_verified": true}
		b := e.browser()
		state := g.start(b, "/goals")
		rec := callback(b, state, "good-code")
		if rec.Code != 302 || rec.Header().Get("Location") != "/goals" || b.sessionEmail() != email {
			t.Fatalf("status=%d location=%s", rec.Code, rec.Header().Get("Location"))
		}
		e.mails.last(t, e.h.mailer.adminTo, "新しいユーザー")
		// 同じ往復はもう使えない（state は1回だけ）。
		if rec := callback(e.browser(), state, "good-code"); rec.Header().Get("Location") != "/login?error=oauth" {
			t.Fatalf("同じ state がもう一度通った: %s", rec.Header().Get("Location"))
		}
	})

	t.Run("06 C4 state が Cookie と違えば断る", func(t *testing.T) {
		g.claims = map[string]any{"sub": "g-" + testHex(6), "email": e.newEmail(), "email_verified": true}
		b := e.browser()
		g.start(b, "/")
		other := e.browser()
		g.start(other, "/")
		// 相手の往復（other の state）を、被害者のブラウザ（b の Cookie）に踏ませる。
		rec := callback(b, other.cookies[oauthCookieName].Value, "good-code")
		if rec.Header().Get("Location") != "/login?error=oauth" || b.sessionEmail() != "" {
			t.Fatalf("location=%s", rec.Header().Get("Location"))
		}
	})

	t.Run("F1 code_verifier が違えば断る", func(t *testing.T) {
		g.claims = map[string]any{"sub": "g-" + testHex(6), "email": e.newEmail(), "email_verified": true}
		b := e.browser()
		state := g.start(b, "/")
		e.fx.exec("UPDATE AuthOAuthState SET codeVerifier = ? WHERE stateHash = ?", "x"+strings.Repeat("y", 42), hashToken(state))
		if rec := callback(b, state, "good-code"); rec.Header().Get("Location") != "/login?error=oauth" || b.sessionEmail() != "" {
			t.Fatalf("location=%s", rec.Header().Get("Location"))
		}
	})

	t.Run("F1 nonce が違えば断る", func(t *testing.T) {
		g.claims = map[string]any{"sub": "g-" + testHex(6), "email": e.newEmail(), "email_verified": true, "nonce": "another-nonce"}
		b := e.browser()
		state := g.start(b, "/")
		if rec := callback(b, state, "good-code"); rec.Header().Get("Location") != "/login?error=oauth" || b.sessionEmail() != "" {
			t.Fatalf("location=%s", rec.Header().Get("Location"))
		}
	})

	t.Run("F2 確認済みの既存の利用者には結びつけ、本人へ知らせる", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		g.claims = map[string]any{"sub": "g-" + testHex(6), "email": email, "email_verified": true}
		b := e.browser()
		callback(b, g.start(b, "/"), "good-code")
		if b.sessionEmail() != email || e.fx.count("SELECT COUNT(*) FROM AuthIdentity WHERE userId = ?", id) != 1 {
			t.Fatal("結びついていない")
		}
		e.mails.last(t, email, "連携しました")
	})

	t.Run("F2 未確認の既存の利用者は、パスワードを消してから結びつける", func(t *testing.T) {
		email := e.newEmail()
		e.browser().do("POST", "/api/auth/sign-up", map[string]string{"email": email, "password": "the attacker passphrase"})
		g.claims = map[string]any{"sub": "g-" + testHex(6), "email": email, "email_verified": true}
		b := e.browser()
		callback(b, g.start(b, "/"), "good-code")
		if b.sessionEmail() != email {
			t.Fatal("ログインしていない")
		}
		expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": "the attacker passphrase"}), 401, "")
	})

	t.Run("F2 プロバイダーが確認済みと示さないメールでは結びつけない", func(t *testing.T) {
		email := e.newEmail()
		e.signUpVerified(email, authTestPassword)
		g.claims = map[string]any{"sub": "g-" + testHex(6), "email": email, "email_verified": false}
		b := e.browser()
		rec := callback(b, g.start(b, "/"), "good-code")
		if rec.Header().Get("Location") != "/login?error=oauth_email" || b.sessionEmail() != "" {
			t.Fatalf("location=%s", rec.Header().Get("Location"))
		}
	})

	t.Run("G3 2段階認証を有効にした人は、外部ログインのあとにもコードを求める", func(t *testing.T) {
		email := e.newEmail()
		e.signUpVerified(email, authTestPassword)
		secret, _ := e.enableMFA(e.signedIn(email, authTestPassword))
		g.claims = map[string]any{"sub": "g-" + testHex(6), "email": email, "email_verified": true}
		b := e.browser()
		rec := callback(b, g.start(b, "/goals"), "good-code")
		if rec.Header().Get("Location") != "/login?mfa=required&callbackURL=%2Fgoals" || b.sessionEmail() != "" {
			t.Fatalf("location=%s", rec.Header().Get("Location"))
		}
		e.clock.Advance(totpPeriod)
		expectStatus(t, b.do("POST", "/api/auth/mfa/verify", map[string]string{"code": totpCode(secret, totpStep(e.clock.Now()))}), 200, "")
		if b.sessionEmail() != email {
			t.Fatal("コードのあとにログインしていない")
		}
	})
}

func TestAuthDBEmailLimitPerRecipient(t *testing.T) {
	// 06 E1：同じ宛先へは1時間に5通まで。超えた分は送らない（応答は変えない）。
	e := newAuthEnv(t)
	email := e.newEmail()
	e.signUpVerified(email, authTestPassword) // 確認メールで1通
	b := e.browser()
	for range 6 {
		expectStatus(t, b.do("POST", "/api/auth/password/forgot", map[string]string{"email": email}), 200, "")
	}
	if got := e.mails.count(email, ""); got != emailPerRecipientPerHour {
		t.Fatalf("送った数 = %d, want %d", got, emailPerRecipientPerHour)
	}
}

func TestAuthDBThrottledEntries(t *testing.T) {
	// H1：推測できる入口ごとに、回数を超えると断る。
	e := newAuthEnv(t)

	t.Run("ログインしていない入口（メールのトークンなど）は IP ごとに10分20回まで", func(t *testing.T) {
		b := e.browser()
		for range throttleAnonymousIP.max {
			expectStatus(t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": strings.Repeat("a", 43)}), 400, "INVALID_TOKEN")
		}
		expectStatus(t, b.do("POST", "/api/auth/verify-email", map[string]string{"token": strings.Repeat("a", 43)}), 429, "TOO_MANY_REQUESTS")
		// 別の IP からは通る（IP ごとに数えている）。
		expectStatus(t, e.browser().do("POST", "/api/auth/verify-email", map[string]string{"token": strings.Repeat("a", 43)}), 400, "INVALID_TOKEN")
	})

	t.Run("再認証（今のパスワードの入れ直し）はアカウントごとに15分10回まで", func(t *testing.T) {
		email := e.newEmail()
		e.signUpVerified(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		for i := range throttleReauthAccount.max {
			expectStatus(t, b.do("POST", "/api/auth/password/change", map[string]string{"currentPassword": fmt.Sprint("wrong passphrase ", i), "newPassword": "a brand new passphrase"}), 400, "INVALID_PASSWORD")
		}
		expectStatus(t, b.do("POST", "/api/auth/password/change", map[string]string{"currentPassword": authTestPassword, "newPassword": "a brand new passphrase"}), 429, "TOO_MANY_REQUESTS")
		dbFixture{t, e.db}.exec("DELETE FROM AuthThrottle WHERE bucket = ?", throttleBucket(throttleReauthAccount, e.userID(email)))
	})

	t.Run("2段階認証のコードは、途中の状態を作り直してもアカウントごとに15分10回まで", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		secret, _ := e.enableMFA(e.signedIn(email, authTestPassword))
		b := e.browser()
		for range 2 {
			b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
			for range mfaChallengeMaxAttempts {
				expectStatus(t, b.do("POST", "/api/auth/mfa/verify", map[string]string{"code": "000000"}), 401, "INVALID_CODE")
			}
		}
		b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
		e.clock.Advance(totpPeriod)
		expectStatus(t, b.do("POST", "/api/auth/mfa/verify", map[string]string{"code": totpCode(secret, totpStep(e.clock.Now()))}), 429, "TOO_MANY_MFA_ATTEMPTS")
		dbFixture{t, e.db}.exec("DELETE FROM AuthThrottle WHERE bucket = ?", throttleBucket(throttleMFAAccount, id))
	})
}

func TestAuthDBEventsAreLogged(t *testing.T) {
	// I1：認証の出来事が、利用者 ID・IP・User-Agent と一緒に1行ずつ出て、パスワード・トークン・コードは出ない。
	var buf bytes.Buffer
	var mu sync.Mutex
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(lockedWriter{&buf, &mu}, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	e := newAuthEnv(t)
	email := e.newEmail()
	e.signUpVerified(email, authTestPassword)
	e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": "not the right passphrase"})
	b := e.signedIn(email, authTestPassword)
	b.do("POST", "/api/auth/password/change", map[string]string{"currentPassword": authTestPassword, "newPassword": "a brand new passphrase"})
	b.do("POST", "/api/auth/password/forgot", map[string]string{"email": email})
	resetToken := e.mails.last(t, email, "パスワードの再設定").token(t)
	b.do("POST", "/api/auth/password/reset", map[string]string{"token": resetToken, "password": authTestPassword})
	b = e.signedIn(email, authTestPassword)
	sessionToken := b.cookies[sessionCookieName].Value
	b.do("POST", "/api/auth/sign-out", map[string]any{})

	mu.Lock()
	out := buf.String()
	mu.Unlock()
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil || entry["event"] == nil {
			continue
		}
		event := entry["event"].(string)
		if event == "sign_in_failure" {
			event += ":" + fmt.Sprint(entry["reason"])
		}
		seen[event] = true
		for _, key := range []string{"userId", "ip", "userAgent", "time"} {
			if _, ok := entry[key]; !ok {
				t.Errorf("%s の行に %s が無い: %s", event, key, line)
			}
		}
	}
	for _, want := range []string{"sign_up", "email_verified", "sign_in_failure:bad_password", "session_created", "sign_in_success", "password_changed", "password_reset", "sign_out"} {
		if !seen[want] {
			t.Errorf("%s がログに無い（出たもの %v）", want, seen)
		}
	}
	for _, secret := range []string{authTestPassword, "not the right passphrase", "a brand new passphrase", resetToken, sessionToken} {
		if strings.Contains(out, secret) {
			t.Errorf("ログに秘密が出ている: %q", secret)
		}
	}
}

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func TestAuthDBTOTPKeyRotation(t *testing.T) {
	// I2：TOTP の秘密を暗号化する鍵を作り直しても、今の利用者は2段階認証でログインでき、秘密は新しい鍵で書き直される。
	e := newAuthEnv(t)
	email := e.newEmail()
	id := e.signUpVerified(email, authTestPassword)
	secret, _ := e.enableMFA(e.signedIn(email, authTestPassword))

	rotated, err := newTOTPKeyring("v1:"+base64.StdEncoding.EncodeToString(randomBytes(32)), "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	e.h.totpKeys = rotated
	e.clock.Advance(totpPeriod)
	b := e.browser()
	b.do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword})
	expectStatus(t, b.do("POST", "/api/auth/mfa/verify", map[string]string{"code": totpCode(secret, totpStep(e.clock.Now()))}), 200, "")
	var sealed string
	if err := e.db.QueryRow("SELECT secret FROM AuthTotp WHERE userId = ?", id).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "v1:") {
		t.Fatalf("新しい鍵で書き直されていない: %s", sealed[:3])
	}
}

func TestAuthDBH2SignInTiming(t *testing.T) {
	// H2 の測り方：存在するメールアドレス（パスワード違い）と、存在しないメールアドレスで、サインインの所要時間の
	// 分布を比べる。時間は機械に左右されて CI では安定しないので、AUTH_TIMING=on のときだけ測って出す。
	if os.Getenv("AUTH_TIMING") != "on" {
		t.Skip("AUTH_TIMING=on のときだけ測る")
	}
	e := newAuthEnv(t)
	email := e.newEmail()
	e.signUpVerified(email, authTestPassword)
	measure := func(target string) []time.Duration {
		var out []time.Duration
		for i := range 40 {
			b := e.browser()
			start := time.Now()
			b.do("POST", "/api/auth/sign-in", map[string]string{"email": target, "password": fmt.Sprint("wrong passphrase ", i)})
			out = append(out, time.Since(start))
			dbFixture{t, e.db}.exec("DELETE FROM AuthThrottle WHERE bucket = ?", throttleBucket(throttleSignInAccount, target))
		}
		slices.Sort(out)
		return out
	}
	existing, missing := measure(email), measure(e.newEmail())
	pct := func(d []time.Duration, p int) time.Duration { return d[len(d)*p/100] }
	t.Logf("サインイン 存在する   p10=%v p50=%v p90=%v", pct(existing, 10), pct(existing, 50), pct(existing, 90))
	t.Logf("サインイン 存在しない p10=%v p50=%v p90=%v", pct(missing, 10), pct(missing, 50), pct(missing, 90))

	// 登録：登録済みのメールアドレスと、新しいメールアドレス。
	signUp := func(next func() string) []time.Duration {
		var out []time.Duration
		for range 40 {
			b := e.browser()
			target := next()
			start := time.Now()
			b.do("POST", "/api/auth/sign-up", map[string]string{"email": target, "password": "another long passphrase"})
			out = append(out, time.Since(start))
			dbFixture{t, e.db}.exec("DELETE FROM EmailSend WHERE recipientHash = ?", recipientHash(target))
		}
		slices.Sort(out)
		return out
	}
	registered, fresh := signUp(func() string { return email }), signUp(e.newEmail)
	t.Logf("登録 登録済み p10=%v p50=%v p90=%v", pct(registered, 10), pct(registered, 50), pct(registered, 90))
	t.Logf("登録 新しい   p10=%v p50=%v p90=%v", pct(fresh, 10), pct(fresh, 50), pct(fresh, 90))
}
