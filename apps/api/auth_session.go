package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
)

// セッション（認証基準 10 の C1〜C5）と Cookie（D1）。
//
// 方式（C1）：中身の無いランダムなトークンを Cookie で運び、状態は DB（AuthSession）に持つ。
// JWT のように中身を持たせないので、行を消せばその時点で使えなくなり（停止・パスワード変更・
// pnpm incident の締め出し）、role・停止・2段階認証の状態は毎回 DB の新しい値で判定する。

// Cookie の名前。__Host- を付けると、ブラウザが Secure・Path=/・Domain 指定なしを強制する（D1）。
// サブドメインから上書き・送信されない。そのかわり www と apex でログインを共有できないので、
// www は apex へ寄せる（JUK-23）。手元の http://localhost でも、ブラウザは localhost を安全な場所として
// 扱うので Secure の Cookie を置ける（E2E の Chromium はこれで手元の http://localhost に入っている）。
const (
	sessionCookieName = "__Host-jm_session"
	// mfaCookieName は、パスワードは合ったが2段階認証がまだの状態（G3）。セッションとは別の Cookie にする。
	mfaCookieName = "__Host-jm_mfa"
	// oauthCookieName は、外部ログインの往復の state（06 C4）。別のサイト（Google・GitHub）から
	// 戻ってくるときに読むので、SameSite=Lax にする（Strict だと戻ってきたときに送られない）。
	oauthCookieName = "__Host-jm_oauth"
)

// sessionPolicy はセッションの期限（C3）。
type sessionPolicy struct {
	// absolute はログインからの上限。使っても延ばさない（延ばし続けられるセッションは、盗まれたら永久に使える）。
	absolute time.Duration
	// idle は使わないときの期限。0 は置かない。
	idle time.Duration
}

// policyFor は利用者の role で期限を決める。
//
//   - 一般の利用者：上限 30 日、使わないときの期限は置かない。受験生が自分のスマホで毎日使う前提で、
//     ログインし直しの手間（Q4）を増やさない。守るもの（学習の記録）に対して、30 日の上限で足りる
//   - 管理者：上限 24 時間、使わないとき 1 時間。管理の権限（全員の個人情報・停止・削除）を持つセッションは、
//     放置された端末や盗まれた Cookie で使える時間を短くする（NIST 800-63B の AAL2 の再認証の期限）
//
// role を変えたら（pnpm admin:grant）、その人のセッションを消して、新しい期限でログインし直させる（C4）。
func policyFor(role string) sessionPolicy {
	if role == "admin" {
		return sessionPolicy{absolute: 24 * time.Hour, idle: time.Hour}
	}
	return sessionPolicy{absolute: 30 * 24 * time.Hour}
}

// touchInterval より前に使ったきりのセッションだけ lastUsedAt を書き直す。リクエストのたびに
// 書き込まないための間引き。使わないときの期限は、そのぶん（最大 1 分）遅れて効く。
const touchInterval = time.Minute

type sessionStore struct {
	db  *sql.DB
	now func() time.Time
}

func (st *sessionStore) clock() time.Time {
	return st.now().UTC().Truncate(time.Millisecond)
}

// create は新しいセッションを作り、Cookie に入れるトークンと期限を返す。
// ログインの成功・2段階認証の完了のたびに呼び、前のトークンを使い続けない（C4）。
func (st *sessionStore) create(ctx context.Context, r *http.Request, userID, role string, mfaVerified bool) (raw string, expiresAt time.Time, err error) {
	now := st.clock()
	policy := policyFor(role)
	raw, hash := newToken()
	expiresAt = now.Add(policy.absolute)
	if err := account.CreateSession(ctx, st.db, account.NewSession{
		TokenHash: hash, UserID: userID, ExpiresAt: expiresAt, IdleTimeout: policy.idle, MFAVerified: mfaVerified,
		IPAddress: truncate(clientIP(r), 64), UserAgent: truncate(r.UserAgent(), 512),
	}, now); err != nil {
		return "", time.Time{}, err
	}
	return raw, expiresAt, nil
}

// load はトークンからセッションを読む。期限（上限・使わないとき）を過ぎていれば (nil, nil)。
func (st *sessionStore) load(ctx context.Context, raw string) (*session, error) {
	hash := hashToken(raw)
	if hash == nil {
		return nil, nil
	}
	now := st.clock()
	// 期限は DB の値とアプリの時刻で比べる（DB の NOW() は接続の時間帯に左右される。時刻を進めるテストもできる）。
	var s session
	var email, role sql.NullString
	var stale bool
	err := st.db.QueryRowContext(ctx,
		"SELECT s.id, s.userId, u.email, u.role, u.bannedAt IS NOT NULL, s.mfaVerifiedAt IS NOT NULL, s.lastUsedAt < ?"+
			" FROM AuthSession AS s JOIN `user` AS u ON u.id = s.userId"+
			" WHERE s.tokenHash = ? AND s.expiresAt > ?"+
			" AND (s.idleTimeoutSeconds IS NULL OR s.lastUsedAt > DATE_SUB(?, INTERVAL s.idleTimeoutSeconds SECOND))",
		now.Add(-touchInterval), hash, now, now,
	).Scan(&s.ID, &s.UserID, &email, &role, &s.Banned, &s.TwoFactorVerified, &stale)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find session: %w", err)
	}
	s.Email = email.String
	s.Role = role.String
	if stale {
		if err := account.TouchSession(ctx, st.db, s.ID, now); err != nil {
			return nil, err
		}
	}
	return &s, nil
}

// revoke は1つのセッションを消す（C5 の「この端末」。ログアウト）。行の作成・消去は持ち主の internal/write/account（JUK-154）。
func (st *sessionStore) revoke(ctx context.Context, sessionID string) error {
	return account.RevokeSession(ctx, st.db, sessionID)
}

// sessionAuth はルーター（router.go）にセッションの読み方を渡す。
type sessionAuth struct {
	store *sessionStore
}

// load はリクエストの Cookie からセッションを読む。ログインしていなければ (nil, nil)。
// 断るかどうか（401・403）はルーター（router.go の requireSession）が決める。
func (a *sessionAuth) load(r *http.Request) (*session, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, nil
	}
	return a.store.load(r.Context(), c.Value)
}

// setSessionCookie はセッションの Cookie を置く。期限の正は DB で、Cookie の寿命は合わせるだけ（C3）。
// SameSite=Strict：別のサイトから来たリクエストには付かない。SPA は HTML の配信にログインが要らないので、
// 別のサイトのリンクから来ても困らない（開いたあとの API は同じサイトから呼ばれる）。
func setSessionCookie(w http.ResponseWriter, raw string, expiresAt, now time.Time) {
	setCookie(w, sessionCookieName, raw, expiresAt.Sub(now), http.SameSiteStrictMode)
}

func setCookie(w http.ResponseWriter, name, value string, maxAge time.Duration, sameSite http.SameSite) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   max(1, int(maxAge/time.Second)),
		HttpOnly: true,
		Secure:   true,
		SameSite: sameSite,
	})
}

func clearCookie(w http.ResponseWriter, name string, sameSite http.SameSite) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: sameSite})
}

// clientIP は接続元の IP。本番と開発の nginx は X-Forwarded-For を接続元（$remote_addr）で上書きして渡すので、
// その値を使う（利用者が送ってきた値は nginx が捨てている）。nginx を通らないとき（テスト）は接続そのものの値。
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// truncate は列の長さに収める。文字の途中で切れたバイトは捨てる（壊れた UTF-8 を DB に渡さない）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
