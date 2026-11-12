package notification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// LINE の連携の読み取り。

// StoredAttempt は残してある LINE Login の試行。Expired は DB の時計（UTC）で期限を過ぎたか。
type StoredAttempt struct {
	Attempt
	Expired bool
}

// IsConnected は、その利用者が LINE と連携しているか。
func IsConnected(ctx context.Context, db *sql.DB, userID string) (bool, error) {
	return exists(ctx, db, "SELECT 1 FROM LineConnection WHERE userId = ? LIMIT 1", userID)
}

// IsLineUserConnected は、その LINE がどれかのアカウントと連携しているか。
func IsLineUserConnected(ctx context.Context, db *sql.DB, lineUserID string) (bool, error) {
	return exists(ctx, db, "SELECT 1 FROM LineConnection WHERE lineUserId = ? LIMIT 1", lineUserID)
}

func exists(ctx context.Context, db *sql.DB, query string, arg any) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx, query, arg).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("line: %w", err)
	}
	return true, nil
}

// FindOAuthAttempt は state で LINE Login の試行を引き当てる。無ければ nil。
func FindOAuthAttempt(ctx context.Context, db *sql.DB, state string) (*StoredAttempt, error) {
	var a StoredAttempt
	err := db.QueryRowContext(ctx,
		`SELECT userId, nonce, codeVerifier, redirectUri, expiresAt <= UTC_TIMESTAMP(3)
		 FROM LineOAuthAttempt WHERE state = ?`, state,
	).Scan(&a.UserID, &a.Nonce, &a.CodeVerifier, &a.RedirectURI, &a.Expired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find line oauth attempt: %w", err)
	}
	return &a, nil
}
