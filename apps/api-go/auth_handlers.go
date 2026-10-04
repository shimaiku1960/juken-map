package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ログイン（/api/auth/*）。Better Auth（Node）が受けていたものを Go で自作した（JUK-115）。
// 判定の基準は dev-standards の targets/10_authentication.md（認証 基準 v1.0）で、コメントの
// A1・B3 などはその項目の記号。06 で始まるものは 06_security.md の項目。
//
// 入口（すべて /api/auth/ の下。書き込みは別のサイトから断る。10 D2）
//
//	GET  session                 今のログイン（無ければ null）
//	POST sign-up                 メール＋パスワードの登録。確認メールを送る
//	POST sign-in                 メール＋パスワードのログイン。2段階認証が要れば途中の状態にする
//	POST sign-out                ログアウト（この端末のセッションを消す）
//	POST verify-email            メールのリンクのトークンでメールアドレスを確認する
//	POST verify-email/resend     確認メールを送り直す
//	POST password/forgot         再設定のメールを送る
//	POST password/reset          メールのリンクのトークンでパスワードを決め直す
//	POST password/change         ログイン中にパスワードを変える（今のパスワードを入れ直す）
//	GET  accounts                ログインの手段（パスワードの有無・連携している外部サービス）
//	POST mfa/setup・mfa/confirm  2段階認証を設定する（auth_mfa.go）
//	POST mfa/verify              ログインの途中で2段階認証のコードを確かめる（auth_mfa.go）
//	POST oauth/{provider}        外部ログインを始める（auth_oauth.go）
//	GET  callback/{provider}     外部ログインから戻ってくる（auth_oauth.go）

const (
	// authBodyLimit は認証の入口で受け取る本文の上限。パスワード（256 バイトまで）より十分大きく、小さく保つ。
	authBodyLimit = 16 << 10
	// verifyEmailTTL・passwordResetTTL はメールで送るトークンの寿命（10 E1）。
	verifyEmailTTL   = 24 * time.Hour
	passwordResetTTL = time.Hour
	// mfaChallengeTTL は、パスワードが合ってから2段階認証のコードを入れるまでの時間（G3）。
	mfaChallengeTTL           = 5 * time.Minute
	tokenPurposeVerifyEmail   = "verify-email"
	tokenPurposePasswordReset = "password-reset"
	// invalidCredentialsMessage はメールアドレスが無いときも、パスワードが違うときも同じ文言にする（06 B4）。
	invalidCredentialsMessage = "メールアドレスまたはパスワードが違います"
	bannedMessage             = "このアカウントは利用を停止されています。"
)

// authHandlers は認証の入口が使うものをまとめる。
type authHandlers struct {
	db        *sql.DB
	sessions  *sessionStore
	hasher    *passwordHasher
	throttle  *throttle
	mailer    *authMailer
	totpKeys  *totpKeyring
	oauth     map[string]*oauthProvider
	webOrigin string
	now       func() time.Time
}

func (h *authHandlers) clock() time.Time {
	return h.now().UTC().Truncate(time.Millisecond)
}

// ---- 利用者 ----

// authUser はログインの判定に使う利用者の値。
type authUser struct {
	ID            string
	Email         string
	Name          string
	Role          string
	EmailVerified bool
	Banned        bool
	MFAEnabled    bool
	PasswordHash  sql.NullString
}

const authUserColumns = "u.id, COALESCE(u.email, ''), COALESCE(u.name, ''), u.role, u.emailVerified, u.bannedAt IS NOT NULL," +
	" EXISTS(SELECT 1 FROM AuthTotp AS t WHERE t.userId = u.id AND t.enabledAt IS NOT NULL), p.hash" +
	" FROM `user` AS u LEFT JOIN AuthPassword AS p ON p.userId = u.id"

func scanAuthUser(row *sql.Row) (*authUser, error) {
	var u authUser
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified, &u.Banned, &u.MFAEnabled, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (h *authHandlers) findUserByEmail(ctx context.Context, email string) (*authUser, error) {
	return scanAuthUser(h.db.QueryRowContext(ctx, "SELECT "+authUserColumns+" WHERE u.email = ?", email))
}

func (h *authHandlers) findUserByID(ctx context.Context, id string) (*authUser, error) {
	return scanAuthUser(h.db.QueryRowContext(ctx, "SELECT "+authUserColumns+" WHERE u.id = ?", id))
}

// normalizeEmail はメールアドレスを小文字にして前後の空白を落とす。保存も検索もこの形で行う。
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validEmail は形だけを見る（届くかどうかは確認メールで確かめる）。
func validEmail(email string) bool {
	local, domain, ok := strings.Cut(email, "@")
	return ok && local != "" && strings.Contains(domain, ".") && !strings.ContainsAny(email, " \t\r\n<>\"") &&
		len(email) <= 191 && !strings.Contains(domain, "@")
}

func newUserID() string {
	return hex.EncodeToString(randomBytes(16))
}

// setPassword はパスワードのハッシュを保存する（無ければ作る）。
func setPassword(ctx context.Context, q execer, userID, hash string, now time.Time) error {
	_, err := q.ExecContext(ctx,
		"INSERT INTO AuthPassword (userId, hash, updatedAt) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE hash = VALUES(hash), updatedAt = VALUES(updatedAt)",
		userID, hash, now)
	return err
}

// execer は *sql.DB と *sql.Tx の両方で使う書き込み。
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ---- メールで送るトークン（10 E1） ----

// issueToken は用途つきのトークンを作る。同じ人・同じ用途の古いものと、期限の切れたものは消す。
func (h *authHandlers) issueToken(ctx context.Context, userID, purpose string, ttl time.Duration) (string, error) {
	now := h.clock()
	if _, err := h.db.ExecContext(ctx, "DELETE FROM AuthToken WHERE userId = ? AND (purpose = ? OR expiresAt <= ?)", userID, purpose, now); err != nil {
		return "", err
	}
	raw, hash := newToken()
	_, err := h.db.ExecContext(ctx,
		"INSERT INTO AuthToken (tokenHash, purpose, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?, ?)",
		hash, purpose, userID, now, now.Add(ttl))
	return raw, err
}

// peekToken はトークンを使わずに持ち主を返す（使う前にパスワードの規則を確かめたいとき）。
func (h *authHandlers) peekToken(ctx context.Context, raw, purpose string) (string, error) {
	hash := hashToken(raw)
	if hash == nil {
		return "", nil
	}
	var userID string
	err := h.db.QueryRowContext(ctx,
		"SELECT userId FROM AuthToken WHERE tokenHash = ? AND purpose = ? AND expiresAt > ?", hash, purpose, h.clock()).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return userID, err
}

// consumeToken はトークンを使う。用途が違う・期限切れ・使用済みなら空を返す。
// 消せたときだけ使えたことにするので、同じトークンが同時に2回送られても1回しか通らない。
func (h *authHandlers) consumeToken(ctx context.Context, raw, purpose string) (string, error) {
	userID, err := h.peekToken(ctx, raw, purpose)
	if err != nil || userID == "" {
		return "", err
	}
	res, err := h.db.ExecContext(ctx,
		"DELETE FROM AuthToken WHERE tokenHash = ? AND purpose = ? AND expiresAt > ?", hashToken(raw), purpose, h.clock())
	if err != nil {
		return "", err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return "", err
	}
	return userID, nil
}

// ---- 応答とログ ----

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}

func writeOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func writeTooMany(w http.ResponseWriter, code string, retryAfter time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
	writeAuthError(w, http.StatusTooManyRequests, code, "試行が多すぎます。しばらく待ってから、もう一度お試しください。")
}

// readAuthJSON は JSON の本文を dst へ読む。JSON でなければ 400 を返して false。
func readAuthJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, ok := readBody(w, r, authBodyLimit)
	if !ok {
		return false
	}
	if !body.parsed || body.text || json.Unmarshal([]byte(body.raw), dst) != nil {
		writeError(w, http.StatusBadRequest, "入力が正しくありません")
		return false
	}
	return true
}

// logAuthEvent は認証の出来事を構造化ログに1行出す（10 I1）。パスワード・トークン・コードは渡さない。
// 失敗の種類（reason）は記録するが、画面には出さない（06 B4）。
func logAuthEvent(r *http.Request, level slog.Level, event, userID string, attrs ...any) {
	args := append([]any{"event", event, "userId", userID, "ip", clientIP(r), "userAgent", truncate(r.UserAgent(), 256)}, attrs...)
	slog.Log(r.Context(), level, "[auth] "+event, args...)
}

// safeRedirectPath はログイン後の戻り先を、自分のオリジンのパスだけに絞る（10 F3）。
// 外部の URL・//evil.example・/\evil.example（ブラウザが // と読む）は既定の行き先に置き換える。
func safeRedirectPath(p, fallback string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") ||
		strings.ContainsAny(p, "\r\n\t") || len(p) > 512 {
		return fallback
	}
	return p
}

// ---- ログインの完了 ----

// startSession はログインを完了させる。新しいセッションを作り（C4）、前のセッション（同じブラウザで
// 別の人・同じ人がログインしていたもの）があれば消す。
func (h *authHandlers) startSession(w http.ResponseWriter, r *http.Request, u *authUser, mfaVerified bool, previous *session) error {
	raw, expiresAt, err := h.sessions.create(r.Context(), r, u.ID, u.Role, mfaVerified)
	if err != nil {
		return err
	}
	if previous != nil {
		if err := h.sessions.revoke(r.Context(), previous.ID); err != nil {
			return err
		}
	}
	setSessionCookie(w, raw, expiresAt, h.clock())
	clearCookie(w, mfaCookieName, http.SameSiteLaxMode)
	logAuthEvent(r, slog.LevelInfo, "session_created", u.ID, "mfa", mfaVerified)
	return nil
}

// startMFAChallenge は「パスワード（か外部ログイン）は通ったが、2段階認証がまだ」の状態を作る（G3）。
// セッションではない一時的な状態で、できるのはコードの確認だけ。
func (h *authHandlers) startMFAChallenge(w http.ResponseWriter, r *http.Request, userID string) error {
	now := h.clock()
	ctx := r.Context()
	if _, err := h.db.ExecContext(ctx, "DELETE FROM AuthMfaChallenge WHERE userId = ? OR expiresAt <= ?", userID, now); err != nil {
		return err
	}
	raw, hash := newToken()
	if _, err := h.db.ExecContext(ctx,
		"INSERT INTO AuthMfaChallenge (tokenHash, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?)",
		hash, userID, now, now.Add(mfaChallengeTTL)); err != nil {
		return err
	}
	setCookie(w, mfaCookieName, raw, mfaChallengeTTL, http.SameSiteLaxMode)
	logAuthEvent(r, slog.LevelInfo, "mfa_challenge_started", userID)
	return nil
}

// ---- 入口 ----

// SessionResponse は GET /api/auth/session の応答。画面（apps/web の lib/auth-client.ts）が読む。
type SessionResponse struct {
	User    SessionUser `json:"user"`
	Session SessionInfo `json:"session"`
}

type SessionUser struct {
	ID               string  `json:"id"`
	Email            string  `json:"email"`
	Name             string  `json:"name"`
	Nickname         *string `json:"nickname"`
	Image            *string `json:"image"`
	Role             string  `json:"role"`
	EmailVerified    bool    `json:"emailVerified"`
	TwoFactorEnabled bool    `json:"twoFactorEnabled"`
	CreatedAt        string  `json:"createdAt"`
}

type SessionInfo struct {
	ID                string `json:"id"`
	ExpiresAt         string `json:"expiresAt"`
	TwoFactorVerified bool   `json:"twoFactorVerified"`
}

// session は GET /api/auth/session。ログインしていなければ null（200）を返す。
func (h *authHandlers) session(w http.ResponseWriter, r *http.Request, s *session) {
	if s == nil || s.Banned {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	var res SessionResponse
	var name sql.NullString
	var createdAt, expiresAt string
	err := h.db.QueryRowContext(r.Context(),
		"SELECT u.id, COALESCE(u.email, ''), u.name, u.nickname, u.image, u.role, u.emailVerified,"+
			" EXISTS(SELECT 1 FROM AuthTotp AS t WHERE t.userId = u.id AND t.enabledAt IS NOT NULL), u.createdAt, s.expiresAt"+
			" FROM AuthSession AS s JOIN `user` AS u ON u.id = s.userId WHERE s.id = ?", s.ID,
	).Scan(&res.User.ID, &res.User.Email, &name, &res.User.Nickname, &res.User.Image, &res.User.Role,
		&res.User.EmailVerified, &res.User.TwoFactorEnabled, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		internalError(w, r, fmt.Errorf("auth session: %w", err))
		return
	}
	res.User.Name = name.String
	res.User.CreatedAt = isoFromDatetime(createdAt)
	res.Session = SessionInfo{ID: s.ID, ExpiresAt: isoFromDatetime(expiresAt), TwoFactorVerified: s.TwoFactorVerified}
	writeJSON(w, http.StatusOK, res)
}

// signUp は POST /api/auth/sign-up。
//
// 応答は、新しいメールアドレスでも登録済みのメールアドレスでも同じ（06 B4・10 H2）。登録済みなら本人へ
// 「登録済みです」と知らせるだけで、アカウントには触らない。どちらの場合もパスワードのハッシュを1回計算し、
// メールは応答のあとに送るので、時間の差も出ない。
//
// まだ確認していないアカウントへの登録し直しは、パスワードを新しいものに置き換えて確認メールを送り直す
// （古い確認のリンクは無効になる）。相手が先に被害者のメールアドレスで登録しておき、被害者が確認した後も
// 相手のパスワードで入れる、という乗っ取りを防ぐため（10 F2 の迷ったら と同じ考え方）。
func (h *authHandlers) signUp(w http.ResponseWriter, r *http.Request, _ *session) {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		CallbackURL string `json:"callbackURL"`
	}
	if !readAuthJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	if !h.allowAnonymous(w, r) {
		return
	}
	email := normalizeEmail(in.Email)
	if !validEmail(email) {
		writeAuthError(w, http.StatusBadRequest, "INVALID_EMAIL", "メールアドレスの形が正しくありません")
		return
	}
	if message, ok := checkNewPassword(in.Password, email); !ok {
		writeAuthError(w, http.StatusBadRequest, "WEAK_PASSWORD", message)
		return
	}
	hash, err := h.hasher.hash(ctx, in.Password)
	if err != nil {
		internalError(w, r, fmt.Errorf("sign-up: %w", err))
		return
	}
	existing, err := h.findUserByEmail(ctx, email)
	if err != nil {
		internalError(w, r, fmt.Errorf("sign-up: %w", err))
		return
	}
	callback := safeRedirectPath(in.CallbackURL, "/dashboard")
	now := h.clock()
	switch {
	case existing == nil:
		id := newUserID()
		tx, err := h.db.BeginTx(ctx, nil)
		if err != nil {
			internalError(w, r, fmt.Errorf("sign-up: %w", err))
			return
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO `user` (id, name, email, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, false, ?, ?)",
			id, email, email, now, now); err != nil {
			internalError(w, r, fmt.Errorf("sign-up: %w", err))
			return
		}
		if err := setPassword(ctx, tx, id, hash, now); err != nil {
			internalError(w, r, fmt.Errorf("sign-up: %w", err))
			return
		}
		if err := tx.Commit(); err != nil {
			internalError(w, r, fmt.Errorf("sign-up: %w", err))
			return
		}
		if err := h.sendVerification(ctx, id, email, callback); err != nil {
			internalError(w, r, fmt.Errorf("sign-up: %w", err))
			return
		}
		logAuthEvent(r, slog.LevelInfo, "sign_up", id)
	case !existing.EmailVerified:
		if err := setPassword(ctx, h.db, existing.ID, hash, now); err != nil {
			internalError(w, r, fmt.Errorf("sign-up: %w", err))
			return
		}
		if err := h.sendVerification(ctx, existing.ID, email, callback); err != nil {
			internalError(w, r, fmt.Errorf("sign-up: %w", err))
			return
		}
		logAuthEvent(r, slog.LevelInfo, "sign_up_unverified_again", existing.ID)
	default:
		h.mailer.sendAlreadyRegistered(email, h.webOrigin)
		logAuthEvent(r, slog.LevelInfo, "sign_up_existing", existing.ID)
	}
	writeOK(w)
}

// sendVerification は確認のトークンを作り、メールを送る（送るのは応答のあと）。
// リンクは確認の画面を開くだけで、トークンを使うのは画面のボタンからの POST（10 E2）。
func (h *authHandlers) sendVerification(ctx context.Context, userID, email, callback string) error {
	raw, err := h.issueToken(ctx, userID, tokenPurposeVerifyEmail, verifyEmailTTL)
	if err != nil {
		return err
	}
	link := h.webOrigin + "/verify-email/confirm?token=" + raw
	if callback != "/dashboard" {
		link += "&callbackURL=" + url.QueryEscape(callback)
	}
	h.mailer.sendVerification(email, link)
	return nil
}

// allowAnonymous はログインしていなくても呼べる入口の、IP 単位の回数制限（H1）。止めたら 429 を返して false。
func (h *authHandlers) allowAnonymous(w http.ResponseWriter, r *http.Request) bool {
	ok, retry, err := h.throttle.hit(r.Context(), throttleAnonymousIP, clientIP(r))
	if err != nil {
		internalError(w, r, fmt.Errorf("throttle: %w", err))
		return false
	}
	if !ok {
		logAuthEvent(r, slog.LevelWarn, "throttled", "", "rule", throttleAnonymousIP.name, "path", r.URL.Path)
		writeTooMany(w, "TOO_MANY_REQUESTS", retry)
		return false
	}
	return true
}

// signIn は POST /api/auth/sign-in。
func (h *authHandlers) signIn(w http.ResponseWriter, r *http.Request, previous *session) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readAuthJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	email := normalizeEmail(in.Email)

	// IP 単位とアカウント単位の両方で数える（06 B4）。デモアカウントはパスワードを画面に載せていて
	// 守る意味が無く、わざと失敗させれば面接官が入れなくなるので、アカウント単位では数えない。
	ok, retry, err := h.throttle.hit(ctx, throttleSignInIP, clientIP(r))
	if err == nil && ok && email != demoEmail {
		ok, retry, err = h.throttle.hit(ctx, throttleSignInAccount, email)
	}
	if err != nil {
		internalError(w, r, fmt.Errorf("sign-in: %w", err))
		return
	}
	if !ok {
		logAuthEvent(r, slog.LevelWarn, "sign_in_failure", "", "reason", "throttled")
		writeTooMany(w, "TOO_MANY_SIGN_IN_ATTEMPTS", retry)
		return
	}

	u, err := h.findUserByEmail(ctx, email)
	if err != nil {
		internalError(w, r, fmt.Errorf("sign-in: %w", err))
		return
	}
	if u == nil || !u.PasswordHash.Valid {
		// 存在しない利用者（とパスワードを持たない利用者）でも、同じだけ計算してから断る（B3）。
		h.hasher.verifyDummy(ctx, in.Password)
		reason, userID := "unknown_email", ""
		if u != nil {
			reason, userID = "no_password", u.ID
		}
		logAuthEvent(r, slog.LevelWarn, "sign_in_failure", userID, "reason", reason)
		writeAuthError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", invalidCredentialsMessage)
		return
	}
	matched, needsRehash, err := h.hasher.verify(ctx, u.PasswordHash.String, in.Password)
	if err != nil {
		internalError(w, r, fmt.Errorf("sign-in: %w", err))
		return
	}
	if !matched {
		logAuthEvent(r, slog.LevelWarn, "sign_in_failure", u.ID, "reason", "bad_password")
		writeAuthError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", invalidCredentialsMessage)
		return
	}
	// パスワードが合ったら数を消す。メール未確認・停止中で断るのも、パスワードを確かめたあとなので同じ扱い。
	if err := h.throttle.clear(ctx, throttleSignInAccount, email); err != nil {
		internalError(w, r, fmt.Errorf("sign-in: %w", err))
		return
	}
	if needsRehash {
		h.rehash(r, u.ID, in.Password)
	}
	if !u.EmailVerified {
		logAuthEvent(r, slog.LevelInfo, "sign_in_failure", u.ID, "reason", "email_not_verified")
		writeAuthError(w, http.StatusForbidden, "EMAIL_NOT_VERIFIED", "メールアドレスの確認が完了していません。確認メールをご確認ください。")
		return
	}
	if u.Banned {
		logAuthEvent(r, slog.LevelWarn, "sign_in_failure", u.ID, "reason", "banned")
		writeAuthError(w, http.StatusForbidden, "ACCOUNT_BANNED", bannedMessage)
		return
	}
	if u.MFAEnabled {
		if err := h.startMFAChallenge(w, r, u.ID); err != nil {
			internalError(w, r, fmt.Errorf("sign-in: %w", err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"mfaRequired": true})
		return
	}
	if err := h.startSession(w, r, u, false, previous); err != nil {
		internalError(w, r, fmt.Errorf("sign-in: %w", err))
		return
	}
	logAuthEvent(r, slog.LevelInfo, "sign_in_success", u.ID, "method", "password")
	writeJSON(w, http.StatusOK, map[string]bool{"mfaRequired": false})
}

// rehash は、合っていたパスワードを今の方式で作り直して保存する（B2）。失敗してもログインは止めない
// （次のログインでまた作り直す）。
func (h *authHandlers) rehash(r *http.Request, userID, password string) {
	hash, err := h.hasher.hash(r.Context(), password)
	if err == nil {
		err = setPassword(r.Context(), h.db, userID, hash, h.clock())
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "[auth] Failed to rehash password.", "userId", userID, "err", err.Error())
		return
	}
	logAuthEvent(r, slog.LevelInfo, "password_rehashed", userID)
}

// signOut は POST /api/auth/sign-out。この端末のセッションを消す（C5 の1）。
func (h *authHandlers) signOut(w http.ResponseWriter, r *http.Request, s *session) {
	if s != nil {
		if err := h.sessions.revoke(r.Context(), s.ID); err != nil {
			internalError(w, r, fmt.Errorf("sign-out: %w", err))
			return
		}
		logAuthEvent(r, slog.LevelInfo, "sign_out", s.UserID)
	}
	clearCookie(w, sessionCookieName, http.SameSiteStrictMode)
	clearCookie(w, mfaCookieName, http.SameSiteLaxMode)
	writeOK(w)
}

// accounts は GET /api/auth/accounts。ログインの手段の一覧（2段階認証の設定の前に、パスワードがあるかを見る）。
func (h *authHandlers) accounts(w http.ResponseWriter, r *http.Request, s *session) {
	if s == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var hasPassword bool
	if err := h.db.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM AuthPassword WHERE userId = ?)", s.UserID).Scan(&hasPassword); err != nil {
		internalError(w, r, fmt.Errorf("auth accounts: %w", err))
		return
	}
	rows, err := h.db.QueryContext(r.Context(), "SELECT provider FROM AuthIdentity WHERE userId = ? ORDER BY provider", s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("auth accounts: %w", err))
		return
	}
	defer rows.Close()
	providers := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			internalError(w, r, fmt.Errorf("auth accounts: %w", err))
			return
		}
		providers = append(providers, p)
	}
	if err := rows.Err(); err != nil {
		internalError(w, r, fmt.Errorf("auth accounts: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hasPassword": hasPassword, "providers": providers})
}

// authConfig は認証の入口の設定（main.go が環境変数から作る）。
type authConfig struct {
	webOrigin       string
	totpKeys        *totpKeyring
	hashConcurrency int
	metrics         *metrics
	adminTo         string
	sender          emailSender
	oauth           map[string]*oauthProvider
	now             func() time.Time
	// async はメールなどを応答のあとに動かす。nil なら goroutine（テストはその場で動かす）。
	async func(func())
}

func newAuthHandlers(db *sql.DB, cfg authConfig) *authHandlers {
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.async == nil {
		cfg.async = func(f func()) { go f() }
	}
	if cfg.metrics == nil {
		cfg.metrics = newMetrics()
	}
	return &authHandlers{
		db:        db,
		sessions:  &sessionStore{db: db, now: cfg.now},
		hasher:    newPasswordHasher(cfg.hashConcurrency),
		throttle:  &throttle{db: db, now: cfg.now},
		mailer:    &authMailer{db: db, sender: cfg.sender, metrics: cfg.metrics, adminTo: cfg.adminTo, now: cfg.now, async: cfg.async},
		totpKeys:  cfg.totpKeys,
		oauth:     cfg.oauth,
		webOrigin: cfg.webOrigin,
		now:       cfg.now,
	}
}
