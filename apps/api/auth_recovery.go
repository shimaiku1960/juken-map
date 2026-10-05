package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

// メールアドレスの確認・パスワードの再設定・変更（認証基準 10 の E1〜E5、06 の B5・B6）。

// verifyEmail は POST /api/auth/verify-email。メールのリンクは確認の画面（/verify-email/confirm）を開くだけで、
// トークンを使うのはその画面のボタンからのこの POST（E2）。メールのセキュリティ製品やプレビューがリンクを
// 先に開いても、トークンは使われない。確認できてもログインはさせない（ログインの画面へ案内する）。
func (h *authHandlers) verifyEmail(w http.ResponseWriter, r *http.Request, _ *session) {
	var in struct {
		Token string `json:"token"`
	}
	if !readAuthJSON(w, r, &in) || !h.allowAnonymous(w, r) {
		return
	}
	ctx := r.Context()
	userID, err := h.store.consumeToken(ctx, in.Token, tokenPurposeVerifyEmail)
	if err != nil {
		internalError(w, r, fmt.Errorf("verify-email: %w", err))
		return
	}
	if userID == "" {
		logAuthEvent(r, slog.LevelWarn, "email_verify_failure", "", "reason", "invalid_token")
		writeAuthError(w, http.StatusBadRequest, "INVALID_TOKEN", "リンクが無効か、期限が切れています。確認メールを送り直してください。")
		return
	}
	now := h.clock()
	first, err := h.store.markEmailVerified(ctx, userID, now)
	if err != nil {
		internalError(w, r, fmt.Errorf("verify-email: %w", err))
		return
	}
	// 初めて確認できたときだけ、運営者へ新しい利用者を知らせる（Better Auth の afterEmailVerification と同じ）。
	if first {
		if u, err := h.store.findUserByID(ctx, userID); err == nil && u != nil {
			h.mailer.notifyAdminOfNewUser(u.Name, u.Email, now)
		}
	}
	logAuthEvent(r, slog.LevelInfo, "email_verified", userID)
	writeOK(w)
}

// resendVerification は POST /api/auth/verify-email/resend。
// 応答は、宛先があってもなくても、確認済みでも同じ。調べて送るのは応答のあと（10 H2）。
func (h *authHandlers) resendVerification(w http.ResponseWriter, r *http.Request, _ *session) {
	var in struct {
		Email       string `json:"email"`
		CallbackURL string `json:"callbackURL"`
	}
	if !readAuthJSON(w, r, &in) || !h.allowAnonymous(w, r) {
		return
	}
	email := normalizeEmail(in.Email)
	callback := safeRedirectPath(in.CallbackURL, "/dashboard")
	h.later(r, "resend-verification", func(ctx context.Context) error {
		u, err := h.store.findUserByEmail(ctx, email)
		if err != nil || u == nil || u.EmailVerified {
			return err
		}
		return h.sendVerification(ctx, u.ID, u.Email, callback)
	})
	writeOK(w)
}

// forgotPassword は POST /api/auth/password/forgot。応答は宛先の有無で変えず、調べて送るのは応答のあと（H2）。
// パスワードを持たない利用者（Google・GitHub だけ）にも送る。パスワードを作る道がこれだけなので（A1 の回復）。
func (h *authHandlers) forgotPassword(w http.ResponseWriter, r *http.Request, _ *session) {
	var in struct {
		Email string `json:"email"`
	}
	if !readAuthJSON(w, r, &in) || !h.allowAnonymous(w, r) {
		return
	}
	email := normalizeEmail(in.Email)
	h.later(r, "forgot-password", func(ctx context.Context) error {
		u, err := h.store.findUserByEmail(ctx, email)
		if err != nil || u == nil || u.Banned {
			return err
		}
		raw, err := h.store.issueToken(ctx, u.ID, tokenPurposePasswordReset, passwordResetTTL)
		if err != nil {
			return err
		}
		h.mailer.sendPasswordReset(u.Email, h.webOrigin+"/reset-password?token="+raw)
		return nil
	})
	writeOK(w)
}

// later は応答を返したあとに f を動かす（メールの async と同じ仕組み）。失敗はログにだけ残す。
func (h *authHandlers) later(r *http.Request, name string, f func(ctx context.Context) error) {
	reqID := requestIDFrom(r.Context())
	h.mailer.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), authEmailTimeout)
		defer cancel()
		if err := f(ctx); err != nil {
			slog.Error("[auth] Background task failed.", "task", name, "reqId", reqID, "err", err.Error())
		}
	})
}

// resetPassword は POST /api/auth/password/reset。
//
// 再設定は「パスワードを変えるだけ」の流れにする（E3）。
//   - 全セッションと、まだ使われていない再設定・確認のトークン、2段階認証の途中の状態を消す
//   - 自動ではログインさせない（2段階認証があれば、それも通ってもらう）
//   - 2段階認証は外さない
//
// メールのリンクを受け取れたことは、そのアドレスを持っていることの確認になるので、未確認なら確認済みにする。
func (h *authHandlers) resetPassword(w http.ResponseWriter, r *http.Request, _ *session) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !readAuthJSON(w, r, &in) || !h.allowAnonymous(w, r) {
		return
	}
	ctx := r.Context()
	invalid := func() {
		logAuthEvent(r, slog.LevelWarn, "password_reset_failure", "", "reason", "invalid_token")
		writeAuthError(w, http.StatusBadRequest, "INVALID_TOKEN", "リンクが無効か、期限が切れています。もう一度、再設定のメールを送ってください。")
	}
	// 規則に合わないパスワードで断るときは、トークンを使わない（直して送り直せるように）。
	userID, err := h.store.peekToken(ctx, in.Token, tokenPurposePasswordReset)
	if err != nil {
		internalError(w, r, fmt.Errorf("reset-password: %w", err))
		return
	}
	if userID == "" {
		invalid()
		return
	}
	u, err := h.store.findUserByID(ctx, userID)
	if err != nil || u == nil {
		internalError(w, r, fmt.Errorf("reset-password: %w (user=%v)", err, u != nil))
		return
	}
	if message, ok := checkNewPassword(in.Password, u.Email); !ok {
		writeAuthError(w, http.StatusBadRequest, "WEAK_PASSWORD", message)
		return
	}
	hash, err := h.hasher.hash(ctx, in.Password)
	if err != nil {
		internalError(w, r, fmt.Errorf("reset-password: %w", err))
		return
	}
	if consumed, err := h.store.consumeToken(ctx, in.Token, tokenPurposePasswordReset); err != nil {
		internalError(w, r, fmt.Errorf("reset-password: %w", err))
		return
	} else if consumed == "" {
		invalid()
		return
	}
	if err := h.store.replacePassword(ctx, u.ID, hash, ""); err != nil {
		internalError(w, r, fmt.Errorf("reset-password: %w", err))
		return
	}
	if _, err := h.store.markEmailVerified(ctx, u.ID, h.clock()); err != nil {
		internalError(w, r, fmt.Errorf("reset-password: %w", err))
		return
	}
	h.mailer.sendPasswordChanged(u.Email, h.webOrigin)
	logAuthEvent(r, slog.LevelInfo, "password_reset", u.ID)
	writeOK(w)
}

// changePassword は POST /api/auth/password/change。今のパスワードを入れ直してもらい（06 B6・10 E4）、
// ほかの端末のセッションをすべて消して、本人へ知らせる。今の端末はログインしたまま。
func (h *authHandlers) changePassword(w http.ResponseWriter, r *http.Request, s *session) {
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !h.requireUser(w, r, s) || !readAuthJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	u, ok := h.reauthenticate(w, r, s, in.CurrentPassword)
	if !ok {
		return
	}
	if message, ok := checkNewPassword(in.NewPassword, u.Email); !ok {
		writeAuthError(w, http.StatusBadRequest, "WEAK_PASSWORD", message)
		return
	}
	hash, err := h.hasher.hash(ctx, in.NewPassword)
	if err != nil {
		internalError(w, r, fmt.Errorf("change-password: %w", err))
		return
	}
	if err := h.store.replacePassword(ctx, u.ID, hash, s.ID); err != nil {
		internalError(w, r, fmt.Errorf("change-password: %w", err))
		return
	}
	h.mailer.sendPasswordChanged(u.Email, h.webOrigin)
	logAuthEvent(r, slog.LevelInfo, "password_changed", u.ID)
	writeOK(w)
}

// requireUser は、ログインが要る認証の入口（パスワードの変更・2段階認証の設定）で、未ログインは 401、
// 停止中とデモアカウントは 403 にする。止めたら false。
func (h *authHandlers) requireUser(w http.ResponseWriter, r *http.Request, s *session) bool {
	switch {
	case s == nil:
		writeError(w, http.StatusUnauthorized, "Unauthorized")
	case s.Banned:
		writeError(w, http.StatusForbidden, bannedMessage)
	case s.Email == demoEmail:
		writeError(w, http.StatusForbidden, "デモアカウントは閲覧専用です")
	default:
		return true
	}
	return false
}

// reauthenticate は重要な操作の直前に、今のパスワードを入れ直してもらう（10 E4）。
// アカウント単位で回数を数える（H1）。合わなければ応答を返して false。
func (h *authHandlers) reauthenticate(w http.ResponseWriter, r *http.Request, s *session, password string) (*authUser, bool) {
	ctx := r.Context()
	ok, retry, err := h.throttle.hit(ctx, throttleReauthAccount, s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("reauth: %w", err))
		return nil, false
	}
	if !ok {
		logAuthEvent(r, slog.LevelWarn, "reauth_failure", s.UserID, "reason", "throttled")
		writeTooMany(w, "TOO_MANY_REQUESTS", retry)
		return nil, false
	}
	u, err := h.store.findUserByID(ctx, s.UserID)
	if err != nil || u == nil {
		internalError(w, r, fmt.Errorf("reauth: %w (user=%v)", err, u != nil))
		return nil, false
	}
	if !u.PasswordHash.Valid {
		writeAuthError(w, http.StatusBadRequest, "PASSWORD_NOT_SET",
			"このアカウントにはパスワードがありません。ログアウトし、「パスワードを忘れた方」からパスワードを作ってください。")
		return nil, false
	}
	matched, needsRehash, err := h.hasher.verify(ctx, u.PasswordHash.String, password)
	if err != nil {
		internalError(w, r, fmt.Errorf("reauth: %w", err))
		return nil, false
	}
	if !matched {
		logAuthEvent(r, slog.LevelWarn, "reauth_failure", u.ID, "reason", "bad_password")
		writeAuthError(w, http.StatusBadRequest, "INVALID_PASSWORD", "今のパスワードが違います")
		return nil, false
	}
	if err := h.throttle.clear(ctx, throttleReauthAccount, s.UserID); err != nil {
		internalError(w, r, fmt.Errorf("reauth: %w", err))
		return nil, false
	}
	if needsRehash {
		h.rehash(r, u.ID, password)
	}
	return u, true
}
