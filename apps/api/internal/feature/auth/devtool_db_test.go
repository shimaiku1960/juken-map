//go:build dbtest

package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
)

// TestAuthDBDevtoolIssues は、開発用の道具（devtool.go）が作ったハッシュ・セッション・トークンを、
// ログインの入口がそのまま受け付けることを確かめる（JUK-143）。seed・E2E・負荷試験はこれらで利用者を用意する。
func TestAuthDBDevtoolIssues(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	email := e.newEmail()
	userID := e.signUpVerified(email, "first passphrase for devtool")

	// HashPassword で置いたパスワードでログインできる。
	const password = "seed passphrase for devtool"
	hash, err := HashPassword(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	if err := account.SetPassword(ctx, e.db, userID, hash, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.signedIn(email, password)

	// IssueSession の Cookie で、ログインした状態になる。
	cookie, err := IssueSession(ctx, e.db, userID, "user", "127.0.0.1", "juken-map-devtool-test")
	if err != nil {
		t.Fatal(err)
	}
	name, value, ok := strings.Cut(cookie, "=")
	if !ok || name != sessionCookieName {
		t.Fatalf("cookie = %q", cookie)
	}
	b := e.browser()
	b.cookies[name] = &http.Cookie{Name: name, Value: value}
	if got := b.sessionEmail(); got != email {
		t.Fatalf("session email = %q, want %q", got, email)
	}

	// IssueEmailToken のトークンで、パスワードを再設定できる。
	token, err := IssueEmailToken(ctx, e.db, userID, tokenPurposePasswordReset)
	if err != nil {
		t.Fatal(err)
	}
	const reset = "reset passphrase for devtool"
	expectStatus(t, e.browser().do("POST", "/api/auth/password/reset", map[string]string{"token": token, "password": reset}), 200, "")
	e.signedIn(email, reset)

	if _, err := IssueEmailToken(ctx, e.db, userID, "unknown"); err == nil {
		t.Fatal("知らない用途を受け付けた")
	}
}
