package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// 開発用の道具（internal/devtool、cmd/devtool）に出す入口（JUK-143）。seed・E2E・負荷試験が、
// ログインと同じ作り方のパスワードのハッシュ・セッション・メールのトークンを使えるようにする。
// 前は db/ の TypeScript が同じ作り方をなぞっていて、ここを変えたときの直し忘れで seed の利用者が
// ログインできなくなるおそれがあった。本番のサーバーと運用のコマンド（cmd/api）からは呼ばない。

// HashPassword はログインと同じ方式・同じ強さで、パスワードのハッシュを PHC 文字列で返す。
func HashPassword(ctx context.Context, password string) (string, error) {
	return newPasswordHasher(1).hash(ctx, password)
}

// IssueSession はログインしたときと同じ期限のセッションを作り、Cookie の「名前=値」を返す。
// 2段階認証は通していない扱いにする（管理の入口は通らない。internal/httpx/router.go）。
func IssueSession(ctx context.Context, db *sql.DB, userID, role, ip, userAgent string) (string, error) {
	st := &sessionStore{db: db, now: time.Now}
	raw, _, err := st.issue(ctx, userID, role, false, ip, userAgent)
	if err != nil {
		return "", err
	}
	return sessionCookieName + "=" + raw, nil
}

// IssueEmailToken は、メールのリンクに載せるトークン（verify-email・password-reset）を、
// メールを送るときと同じ寿命で発行する。メールは送らない。
func IssueEmailToken(ctx context.Context, db *sql.DB, userID, purpose string) (string, error) {
	var ttl time.Duration
	switch purpose {
	case tokenPurposeVerifyEmail:
		ttl = verifyEmailTTL
	case tokenPurposePasswordReset:
		ttl = passwordResetTTL
	default:
		return "", fmt.Errorf("知らない用途です: %s（%s・%s）", purpose, tokenPurposeVerifyEmail, tokenPurposePasswordReset)
	}
	st := &authStore{db: db, now: time.Now}
	return st.issueToken(ctx, userID, purpose, ttl)
}
