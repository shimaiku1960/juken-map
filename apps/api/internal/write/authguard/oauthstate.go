package authguard

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// 外部ログイン（F1・F2）を始めてから戻ってくるまでの state。DB には state の SHA-256 だけを残す。

// OAuthStateTTL は state の寿命。ブラウザに持たせる cookie の寿命も同じにする。
const OAuthStateTTL = 10 * time.Minute

// OAuthState は外部ログインを始めたときに残し、戻ってきたときに使う値。
type OAuthState struct {
	Verifier, Nonce, RedirectTo string
}

// SaveOAuthState は外部ログインを始めるときの state を保存する。期限の切れたものは消す。
func SaveOAuthState(ctx context.Context, db *sql.DB, stateHash []byte, provider string, s OAuthState, now time.Time) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM AuthOAuthState WHERE expiresAt <= ?", now); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx,
		"INSERT INTO AuthOAuthState (stateHash, provider, codeVerifier, nonce, redirectTo, createdAt, expiresAt) VALUES (?, ?, ?, ?, ?, ?, ?)",
		stateHash, provider, s.Verifier, s.Nonce, s.RedirectTo, now, now.Add(OAuthStateTTL))
	return err
}

// ConsumeOAuthState は state の行を読んで消す（1回だけ使える）。無い・期限切れ・プロバイダーが違う・
// 同時に届いた別のリクエストが先に消した、のどれでも nil を返す。
func ConsumeOAuthState(ctx context.Context, db *sql.DB, stateHash []byte, provider string, now time.Time) (*OAuthState, error) {
	var s OAuthState
	err := db.QueryRowContext(ctx,
		"SELECT codeVerifier, nonce, redirectTo FROM AuthOAuthState WHERE stateHash = ? AND provider = ? AND expiresAt > ?",
		stateHash, provider, now).Scan(&s.Verifier, &s.Nonce, &s.RedirectTo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	res, err := db.ExecContext(ctx, "DELETE FROM AuthOAuthState WHERE stateHash = ?", stateHash)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, nil
	}
	return &s, nil
}
