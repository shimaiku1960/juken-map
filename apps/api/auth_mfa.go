package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// 2段階認証の入口（認証基準 10 の G1〜G4）。TOTP の計算と暗号化は auth_totp.go。
//
// 「この端末では省略する」（信頼済みの端末）は提供しない（G4 の迷ったら）。2段階認証を求めるのは
// 有効にした人（実質は管理者）だけで、管理者には省略を提供しないため。端末を失ったときは、予備コードか、
// 手順書（docs/incident-response.md の pnpm incident reset-2fa）で設定前に戻す。

// mfaSetup は POST /api/auth/mfa/setup。今のパスワードを入れ直してもらい（E4）、秘密と予備コードを作る。
// この時点ではまだ有効にしない。認証アプリのコードを1回確かめてから有効にする（mfaConfirm、G1）。
func (h *authHandlers) mfaSetup(w http.ResponseWriter, r *http.Request, s *session) {
	var in struct {
		Password string `json:"password"`
	}
	if !h.requireUser(w, r, s) || !readAuthJSON(w, r, &in) {
		return
	}
	u, ok := h.reauthenticate(w, r, s, in.Password)
	if !ok {
		return
	}
	if u.MFAEnabled {
		writeAuthError(w, http.StatusConflict, "MFA_ALREADY_ENABLED", "2段階認証はすでに有効です")
		return
	}
	secret := randomBytes(totpSecretSize)
	sealed, err := h.totpKeys.seal(secret, u.ID)
	if err != nil {
		internalError(w, r, fmt.Errorf("mfa setup: %w", err))
		return
	}
	codes, hashes := newBackupCodes()
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(w, r, fmt.Errorf("mfa setup: %w", err))
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO AuthTotp (userId, secret, createdAt, enabledAt, lastUsedStep) VALUES (?, ?, ?, NULL, NULL)
		 ON DUPLICATE KEY UPDATE secret = VALUES(secret), createdAt = VALUES(createdAt), enabledAt = NULL, lastUsedStep = NULL`,
		u.ID, sealed, h.clock()); err != nil {
		internalError(w, r, fmt.Errorf("mfa setup: %w", err))
		return
	}
	// 作り直したら古い予備コードは全部無効になる（G2）。
	if _, err := tx.ExecContext(ctx, "DELETE FROM AuthBackupCode WHERE userId = ?", u.ID); err != nil {
		internalError(w, r, fmt.Errorf("mfa setup: %w", err))
		return
	}
	for _, hash := range hashes {
		if _, err := tx.ExecContext(ctx, "INSERT INTO AuthBackupCode (userId, codeHash) VALUES (?, ?)", u.ID, hash); err != nil {
			internalError(w, r, fmt.Errorf("mfa setup: %w", err))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		internalError(w, r, fmt.Errorf("mfa setup: %w", err))
		return
	}
	logAuthEvent(r, slog.LevelInfo, "mfa_setup_started", u.ID)
	// 予備コードを見せるのは、この応答の1回だけ（DB にはハッシュしか無い）。
	writeJSON(w, http.StatusOK, map[string]any{"totpURI": totpURI(secret, u.Email), "backupCodes": codes})
}

// mfaConfirm は POST /api/auth/mfa/confirm。認証アプリのコードを1回確かめてから有効にし、
// 2段階認証を通したセッションに作り直す（C4）。本人へ知らせる（E5）。
func (h *authHandlers) mfaConfirm(w http.ResponseWriter, r *http.Request, s *session) {
	var in struct {
		Code string `json:"code"`
	}
	if !h.requireUser(w, r, s) || !readAuthJSON(w, r, &in) || !h.allowMFAAttempt(w, r, s.UserID) {
		return
	}
	ctx := r.Context()
	var sealed string
	err := h.db.QueryRowContext(ctx, "SELECT secret FROM AuthTotp WHERE userId = ? AND enabledAt IS NULL", s.UserID).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		writeAuthError(w, http.StatusBadRequest, "MFA_NOT_PENDING", "2段階認証の設定を、最初から始めてください")
		return
	}
	if err != nil {
		internalError(w, r, fmt.Errorf("mfa confirm: %w", err))
		return
	}
	secret, err := h.totpKeys.open(sealed, s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("mfa confirm: %w", err))
		return
	}
	now := h.clock()
	step, ok := matchTOTP(secret, in.Code, now, nil)
	if !ok {
		logAuthEvent(r, slog.LevelWarn, "mfa_failure", s.UserID, "reason", "bad_code", "stage", "setup")
		writeAuthError(w, http.StatusBadRequest, "INVALID_CODE", "コードが正しくありません")
		return
	}
	res, err := h.db.ExecContext(ctx, "UPDATE AuthTotp SET enabledAt = ?, lastUsedStep = ? WHERE userId = ? AND enabledAt IS NULL", now, step, s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("mfa confirm: %w", err))
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		writeAuthError(w, http.StatusBadRequest, "MFA_NOT_PENDING", "2段階認証の設定を、最初から始めてください")
		return
	}
	u, err := h.findUserByID(ctx, s.UserID)
	if err != nil || u == nil {
		internalError(w, r, fmt.Errorf("mfa confirm: %w (user=%v)", err, u != nil))
		return
	}
	if err := h.throttle.clear(ctx, throttleMFAAccount, u.ID); err != nil {
		internalError(w, r, fmt.Errorf("mfa confirm: %w", err))
		return
	}
	if err := h.startSession(w, r, u, true, s); err != nil {
		internalError(w, r, fmt.Errorf("mfa confirm: %w", err))
		return
	}
	h.mailer.sendMFAEnabled(u.Email, h.webOrigin)
	logAuthEvent(r, slog.LevelInfo, "mfa_enabled", u.ID)
	writeOK(w)
}

// mfaVerify は POST /api/auth/mfa/verify。ログインの途中（パスワードか外部ログインは通った状態）で、
// 認証アプリのコードか予備コードを確かめ、通ったら2段階認証を済ませたセッションを作る（G3・C4）。
// previous は同じブラウザに残っていた前のセッション（あれば、ログインの完了で消す。C4）。
func (h *authHandlers) mfaVerify(w http.ResponseWriter, r *http.Request, previous *session) {
	var in struct {
		Code   string `json:"code"`
		Method string `json:"method"`
	}
	if !readAuthJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	expired := func() {
		clearCookie(w, mfaCookieName, http.SameSiteLaxMode)
		writeAuthError(w, http.StatusUnauthorized, "MFA_CHALLENGE_EXPIRED", "時間が経ったか、試行が多すぎます。最初からログインし直してください。")
	}
	c, err := r.Cookie(mfaCookieName)
	hash := []byte(nil)
	if err == nil {
		hash = hashToken(c.Value)
	}
	if hash == nil {
		expired()
		return
	}
	// 途中の状態1つで試せる数を数える（H1）。数えてから確かめるので、同時に送られても上限を超えない。
	now := h.clock()
	res, err := h.db.ExecContext(ctx,
		"UPDATE AuthMfaChallenge SET attempts = attempts + 1 WHERE tokenHash = ? AND expiresAt > ? AND attempts < ?",
		hash, now, mfaChallengeMaxAttempts)
	if err != nil {
		internalError(w, r, fmt.Errorf("mfa verify: %w", err))
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		logAuthEvent(r, slog.LevelWarn, "mfa_failure", "", "reason", "challenge_expired")
		expired()
		return
	}
	var userID string
	if err := h.db.QueryRowContext(ctx, "SELECT userId FROM AuthMfaChallenge WHERE tokenHash = ?", hash).Scan(&userID); err != nil {
		internalError(w, r, fmt.Errorf("mfa verify: %w", err))
		return
	}
	if !h.allowMFAAttempt(w, r, userID) {
		return
	}

	var ok bool
	switch in.Method {
	case "backup":
		ok, err = h.useBackupCode(r, userID, in.Code)
	default:
		ok, err = h.useTOTP(r, userID, in.Code)
	}
	if err != nil {
		internalError(w, r, fmt.Errorf("mfa verify: %w", err))
		return
	}
	if !ok {
		logAuthEvent(r, slog.LevelWarn, "mfa_failure", userID, "reason", "bad_code", "method", in.Method)
		writeAuthError(w, http.StatusUnauthorized, "INVALID_CODE", "コードが正しくありません")
		return
	}

	if _, err := h.db.ExecContext(ctx, "DELETE FROM AuthMfaChallenge WHERE userId = ?", userID); err != nil {
		internalError(w, r, fmt.Errorf("mfa verify: %w", err))
		return
	}
	if err := h.throttle.clear(ctx, throttleMFAAccount, userID); err != nil {
		internalError(w, r, fmt.Errorf("mfa verify: %w", err))
		return
	}
	u, err := h.findUserByID(ctx, userID)
	if err != nil || u == nil {
		internalError(w, r, fmt.Errorf("mfa verify: %w (user=%v)", err, u != nil))
		return
	}
	// 途中で停止されたかもしれないので、セッションを作る直前にもう一度見る。
	if u.Banned {
		clearCookie(w, mfaCookieName, http.SameSiteLaxMode)
		writeAuthError(w, http.StatusForbidden, "ACCOUNT_BANNED", bannedMessage)
		return
	}
	if err := h.startSession(w, r, u, true, previous); err != nil {
		internalError(w, r, fmt.Errorf("mfa verify: %w", err))
		return
	}
	var remaining int
	if err := h.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM AuthBackupCode WHERE userId = ?", userID).Scan(&remaining); err != nil {
		internalError(w, r, fmt.Errorf("mfa verify: %w", err))
		return
	}
	method := "totp"
	if in.Method == "backup" {
		method = "backup"
	}
	logAuthEvent(r, slog.LevelInfo, "sign_in_success", userID, "method", "mfa:"+method)
	// 予備コードの残りの数を本人が分かるようにする（G2）。
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "backupCodesRemaining": remaining})
}

// allowMFAAttempt は2段階認証のコードの、アカウント単位の回数制限（H1）。止めたら 429 を返して false。
func (h *authHandlers) allowMFAAttempt(w http.ResponseWriter, r *http.Request, userID string) bool {
	ok, retry, err := h.throttle.hit(r.Context(), throttleMFAAccount, userID)
	if err != nil {
		internalError(w, r, fmt.Errorf("mfa throttle: %w", err))
		return false
	}
	if !ok {
		logAuthEvent(r, slog.LevelWarn, "mfa_failure", userID, "reason", "throttled")
		writeTooMany(w, "TOO_MANY_MFA_ATTEMPTS", retry)
		return false
	}
	return true
}

// useTOTP は認証アプリのコードを確かめ、通ったステップを使用済みにする。同じコード（同じステップ以前）は
// 二度と通さない。使用済みにするのは条件つきの UPDATE なので、同じコードを同時に2回送っても1回しか通らない。
func (h *authHandlers) useTOTP(r *http.Request, userID, code string) (bool, error) {
	ctx := r.Context()
	var sealed string
	var last sql.NullInt64
	err := h.db.QueryRowContext(ctx, "SELECT secret, lastUsedStep FROM AuthTotp WHERE userId = ? AND enabledAt IS NOT NULL", userID).Scan(&sealed, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	secret, err := h.totpKeys.open(sealed, userID)
	if err != nil {
		return false, err
	}
	var lastStep *int64
	if last.Valid {
		lastStep = &last.Int64
	}
	step, ok := matchTOTP(secret, code, h.clock(), lastStep)
	if !ok {
		return false, nil
	}
	res, err := h.db.ExecContext(ctx,
		"UPDATE AuthTotp SET lastUsedStep = ? WHERE userId = ? AND (lastUsedStep IS NULL OR lastUsedStep < ?)", step, userID, step)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return false, nil
	}
	// 古い版の鍵で暗号化されていたら、今の版で書き直す（I2）。失敗してもログインは止めない。
	if h.totpKeys.needsReseal(sealed) {
		if resealed, err := h.totpKeys.seal(secret, userID); err == nil {
			_, err = h.db.ExecContext(ctx, "UPDATE AuthTotp SET secret = ? WHERE userId = ?", resealed, userID)
			if err != nil {
				slog.ErrorContext(ctx, "[auth] Failed to reseal TOTP secret.", "userId", userID, "err", err.Error())
			}
		}
	}
	return true, nil
}

// useBackupCode は予備コードを確かめ、通ったら消す（1回だけ使える。G2）。
func (h *authHandlers) useBackupCode(r *http.Request, userID, code string) (bool, error) {
	res, err := h.db.ExecContext(r.Context(), "DELETE FROM AuthBackupCode WHERE userId = ? AND codeHash = ?", userID, hashBackupCode(code))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
