package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// 本人の退会（06 G3、JUK-123）。管理者の削除（admin_users.go の deleteUser）と同じ消し方で、
// 利用者とぶら下がるデータをすべて消す。取り消せないので、直前に本人であることを確かめ直す。
//
//   - パスワードがある人は、今のパスワード（reauthenticate と同じ回数制限）
//   - パスワードが無い人（Google・GitHub のログインだけ）は、メールアドレスの打ち込み。パスワードの代わりに
//     確かめられるものが無いため、乗っ取ったセッションでも画面に出ているメールアドレスを打てば通る。
//     2段階認証を有効にしていれば、下のコードが本人の確認になる
//   - 2段階認証を有効にしている人は、さらに認証アプリのコードか予備コード
//
// 管理者は退会できない（最後の管理者が消えて誰も入れなくなるのを防ぐ。先に pnpm admin:grant --revoke で外す）。
// デモアカウントは requireUser が断る。

// deleteAccount は POST /api/auth/delete-account。
func (h *authHandlers) deleteAccount(w http.ResponseWriter, r *http.Request, s *session) {
	var in struct {
		Password string `json:"password"`
		Email    string `json:"email"`
		Code     string `json:"code"`
		Method   string `json:"method"`
	}
	if !h.requireUser(w, r, s) || !readAuthJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	u, err := h.findUserByID(ctx, s.UserID)
	if err != nil || u == nil {
		internalError(w, r, fmt.Errorf("delete-account: %w (user=%v)", err, u != nil))
		return
	}
	if u.Role == "admin" {
		writeAuthError(w, http.StatusConflict, "ADMIN_CANNOT_DELETE",
			"管理者のアカウントは退会できません。先に管理者の権限を外してください。")
		return
	}

	if u.PasswordHash.Valid {
		if _, ok := h.reauthenticate(w, r, s, in.Password); !ok {
			return
		}
	} else if !h.confirmEmail(w, r, u, in.Email) {
		return
	}

	if u.MFAEnabled {
		if strings.TrimSpace(in.Code) == "" {
			writeAuthError(w, http.StatusBadRequest, "MFA_CODE_REQUIRED", "2段階認証のコードを入力してください")
			return
		}
		if !h.allowMFAAttempt(w, r, u.ID) {
			return
		}
		var ok bool
		if in.Method == "backup" {
			ok, err = h.useBackupCode(r, u.ID, in.Code)
		} else {
			ok, err = h.useTOTP(r, u.ID, in.Code)
		}
		if err != nil {
			internalError(w, r, fmt.Errorf("delete-account: %w", err))
			return
		}
		if !ok {
			logAuthEvent(r, slog.LevelWarn, "mfa_failure", u.ID, "reason", "bad_code", "method", in.Method, "action", "delete_account")
			// ログインしたままの操作なので 401 にしない（画面がログアウトしたと取り違えないように）。
			writeAuthError(w, http.StatusBadRequest, "INVALID_CODE", "コードが正しくありません")
			return
		}
	}

	if err := inTx(ctx, h.db, func(tx *sql.Tx) error {
		return deleteUserAndData(ctx, tx, u.ID)
	}); err != nil {
		internalError(w, r, fmt.Errorf("delete-account: %w", err))
		return
	}
	// 回数制限の数は宛先・利用者の SHA-256 で持っていて、窓が過ぎれば消える。消し忘れても本人に戻らない。
	clearCookie(w, sessionCookieName, http.SameSiteStrictMode)
	clearCookie(w, mfaCookieName, http.SameSiteLaxMode)
	if u.Email != "" {
		h.mailer.sendAccountDeleted(u.Email, h.webOrigin)
	}
	logAuthEvent(r, slog.LevelInfo, "account_deleted", u.ID)
	writeOK(w)
}

// confirmEmail はパスワードの無い人の確認。打ち込んだメールアドレスが本人のものと一致しなければ断る。
// 当てずっぽうを繰り返せないよう、再認証と同じ回数制限を通す。
func (h *authHandlers) confirmEmail(w http.ResponseWriter, r *http.Request, u *authUser, typed string) bool {
	ctx := r.Context()
	ok, retry, err := h.throttle.hit(ctx, throttleReauthAccount, u.ID)
	if err != nil {
		internalError(w, r, fmt.Errorf("delete-account: %w", err))
		return false
	}
	if !ok {
		logAuthEvent(r, slog.LevelWarn, "reauth_failure", u.ID, "reason", "throttled")
		writeTooMany(w, "TOO_MANY_REQUESTS", retry)
		return false
	}
	// メールの無い利用者は、空文字どうしで一致してしまうので断る（管理者の削除と同じ）。
	if u.Email == "" || normalizeEmail(typed) != normalizeEmail(u.Email) {
		logAuthEvent(r, slog.LevelWarn, "reauth_failure", u.ID, "reason", "email_mismatch")
		writeAuthError(w, http.StatusBadRequest, "EMAIL_MISMATCH", "メールアドレスが一致しません")
		return false
	}
	if err := h.throttle.clear(ctx, throttleReauthAccount, u.ID); err != nil {
		internalError(w, r, fmt.Errorf("delete-account: %w", err))
		return false
	}
	return true
}

// deleteUserAndData は利用者を消す。本人の退会と管理者の削除の両方が使う。
//
// 利用者を指す表は、すべて外部キーの ON DELETE CASCADE で一緒に消える。すべての表に本人の行が
// 残らないことは TestAuthDBDeleteLeavesNoRows が確かめる。
func deleteUserAndData(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM `user` WHERE id = ?", id)
	return err
}
