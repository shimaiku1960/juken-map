package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Better Auth がログイン時に発行する Cookie の名前。HTTPS の本番では前に __Secure- が付く。
var sessionCookieNames = []string{"better-auth.session_token", "__Secure-better-auth.session_token"}

// sessionAuth は Better Auth（Node 側）が発行したセッション Cookie を確かめる。
// 発行はしない。同じ DB の session テーブルと、同じ BETTER_AUTH_SECRET を使う。
type sessionAuth struct {
	db     *sql.DB
	secret []byte
}

// load はリクエストの Cookie からセッションを読む。ログインしていなければ (nil, nil)。
// 断るかどうか（401・403）はルーター（router.go の requireSession）が決める。
func (a *sessionAuth) load(r *http.Request) (*session, error) {
	token, ok := a.tokenFromCookie(r)
	if !ok {
		return nil, nil
	}

	// Better Auth と同じく、session と user を JOIN して1回で引く。
	// 有効期限は DB の値（UTC）と比べるので、UTC_TIMESTAMP を使う（NOW() は DB の時間帯に依る）。
	// role は後から足した列なので、古い利用者では NULL のことがある（NULL は一般の利用者）。
	var s session
	var role sql.NullString
	err := a.db.QueryRowContext(r.Context(),
		"SELECT s.userId, u.email, u.role, u.bannedAt IS NOT NULL"+
			" FROM session AS s JOIN `user` AS u ON u.id = s.userId"+
			" WHERE s.token = ? AND s.expiresAt > UTC_TIMESTAMP(3)",
		token,
	).Scan(&s.UserID, &s.Email, &role, &s.Banned)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find session: %w", err)
	}
	s.Role = role.String
	return &s, nil
}

// tokenFromCookie は Cookie から署名を確かめたセッショントークンを取り出す。
//
// Cookie の値は encodeURIComponent("トークン.署名")。署名はトークンを BETTER_AUTH_SECRET で
// HMAC-SHA256 したものの base64（better-call の signCookieValue）。署名が合わなければ、
// DB を引く前に断る（ありもしないトークンで DB を叩かせない）。
func (a *sessionAuth) tokenFromCookie(r *http.Request) (string, bool) {
	for _, name := range sessionCookieNames {
		c, err := r.Cookie(name)
		if err != nil {
			continue
		}
		return verifySignedValue(c.Value, a.secret)
	}
	return "", false
}

func verifySignedValue(raw string, secret []byte) (string, bool) {
	// base64 の + や / は %2B・%2F になっている。QueryUnescape だと + が空白になるので使わない。
	value, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}
	i := strings.LastIndexByte(value, '.')
	if i < 1 {
		return "", false
	}
	token, signature := value[:i], value[i+1:]
	// 32バイトの HMAC を base64 にすると必ず44文字で = で終わる。better-call も同じ確かめ方をする。
	if len(signature) != 44 || !strings.HasSuffix(signature, "=") {
		return "", false
	}
	got, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(token))
	// == で比べると、先頭から何バイト合っているかが応答時間に出る。hmac.Equal は常に同じ時間で比べる。
	if !hmac.Equal(got, mac.Sum(nil)) {
		return "", false
	}
	return token, true
}
