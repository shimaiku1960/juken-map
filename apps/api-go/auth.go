package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"log/slog"
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

// userHandler はログイン済みの利用者の ID を受け取るハンドラ。
// Fastify では request.session に載せていたものを、引数で渡す。
type userHandler func(w http.ResponseWriter, r *http.Request, userID string)

// requireUser はログインしていなければ 401、停止された利用者なら 403 を返し、
// 通ったときだけ next を呼ぶ。Node 側の requireSession（apps/api/src/context.ts）にあたる。
func (a *sessionAuth) requireUser(next userHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := a.tokenFromCookie(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		// Better Auth と同じく、session と user を JOIN して1回で引く。
		// 有効期限は DB の値（UTC）と比べるので、UTC_TIMESTAMP を使う（NOW() は DB の時間帯に依る）。
		var userID string
		var banned bool
		err := a.db.QueryRowContext(r.Context(),
			"SELECT s.userId, u.bannedAt IS NOT NULL"+
				" FROM session AS s JOIN `user` AS u ON u.id = s.userId"+
				" WHERE s.token = ? AND s.expiresAt > UTC_TIMESTAMP(3)",
			token,
		).Scan(&userID, &banned)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		if err != nil {
			slog.Error("find session", "err", err)
			writeError(w, http.StatusInternalServerError, "Internal Server Error")
			return
		}
		if banned {
			writeError(w, http.StatusForbidden, "このアカウントは利用を停止されています。")
			return
		}

		next(w, r, userID)
	})
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
