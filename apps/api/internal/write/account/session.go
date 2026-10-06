package account

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// セッション（認証基準 10 の C1〜C5）の行。トークンを作る・期限を決める・Cookie に入れるのは入口（internal/feature/auth/session.go）。

// NewSession は作るセッション。TokenHash はトークンの SHA-256（トークンそのものは DB に残さない）。
type NewSession struct {
	TokenHash []byte
	UserID    string
	ExpiresAt time.Time
	// IdleTimeout は使わないときの期限。0 は置かない。
	IdleTimeout time.Duration
	MFAVerified bool
	IPAddress   string
	UserAgent   string
}

// CreateSession はセッションを1つ作る。その前に、期限の切れたセッションを少し消す（ログインのたびに最大 100 行）。
func CreateSession(ctx context.Context, db *sql.DB, s NewSession, now time.Time) error {
	if err := sweepSessions(ctx, db, now); err != nil {
		return err
	}
	var idle any
	if s.IdleTimeout > 0 {
		idle = int64(s.IdleTimeout / time.Second)
	}
	var mfaAt any
	if s.MFAVerified {
		mfaAt = now
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	_, err := db.ExecContext(ctx,
		`INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, idleTimeoutSeconds, lastUsedAt, mfaVerifiedAt, ipAddress, userAgent)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hex.EncodeToString(id), s.TokenHash, s.UserID, now, s.ExpiresAt, idle, now, mfaAt, s.IPAddress, s.UserAgent)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// TouchSession は使った時刻を書き直す（使わないときの期限を延ばす）。毎回は呼ばず、入口が間引く。
func TouchSession(ctx context.Context, db *sql.DB, sessionID string, now time.Time) error {
	if _, err := db.ExecContext(ctx, "UPDATE AuthSession SET lastUsedAt = ? WHERE id = ?", now, sessionID); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

// RevokeSession は1つのセッションを消す（C5 の「この端末」。ログアウト）。
func RevokeSession(ctx context.Context, db *sql.DB, sessionID string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM AuthSession WHERE id = ?", sessionID)
	return err
}

// RevokeUserSessions はその人のセッションをすべて消し、消した数を返す（C5 の「ある利用者の全端末」）。
// 止めはしないので、パスワードを知っていればまた入れる。audit の扱いは Suspend と同じ（記録が書けなければ消さない）。
func RevokeUserSessions(ctx context.Context, db *sql.DB, userID string, now time.Time, audit *OpsAudit) (int64, error) {
	var removed int64
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		var err error
		if removed, err = deleteUserSessions(ctx, tx, userID); err != nil {
			return err
		}
		return audit.insert(ctx, tx, "revoke", userID, map[string]any{"sessionsRemoved": removed}, now)
	})
	if err != nil {
		return 0, err
	}
	return removed, audit.prune(ctx, db, now)
}

// RevokeAllSessions は全員のセッションを消す（C5 の4）。全員がログインし直しになる。
func RevokeAllSessions(ctx context.Context, db *sql.DB, now time.Time, audit *OpsAudit) (int64, error) {
	var removed int64
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM AuthSession")
		if err != nil {
			return err
		}
		if removed, err = res.RowsAffected(); err != nil {
			return err
		}
		return audit.insert(ctx, tx, "revoke-all", "", map[string]any{"sessionsRemoved": removed}, now)
	})
	if err != nil {
		return 0, err
	}
	return removed, audit.prune(ctx, db, now)
}

// Admin は RevokeAdminSessions でセッションを消した管理者。Email はメールアドレスが無ければ空。
type Admin struct {
	ID, Email string
}

// RevokeAdminSessions は管理者全員のセッションを消す。消した管理者（メールアドレス順）と、消したセッションの数を返す。
// 管理者の行を読んでロックしてから消すので、途中で管理者になった人の分を取りこぼさない。
func RevokeAdminSessions(ctx context.Context, db *sql.DB, now time.Time, audit *OpsAudit) ([]Admin, int64, error) {
	var admins []Admin
	var removed int64
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT id, email FROM `user` WHERE role = 'admin' ORDER BY email ASC FOR SHARE")
		if err != nil {
			return err
		}
		for rows.Next() {
			var a Admin
			var email sql.NullString
			if err := rows.Scan(&a.ID, &email); err != nil {
				rows.Close()
				return err
			}
			a.Email = email.String
			admins = append(admins, a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		ids := []string{}
		for _, a := range admins {
			n, err := deleteUserSessions(ctx, tx, a.ID)
			if err != nil {
				return err
			}
			removed += n
			ids = append(ids, a.ID)
		}
		// 対象が複数なので、targetId は空にして detail に userId を並べる（メールアドレスは残さない）。
		return audit.insert(ctx, tx, "revoke-admins", "", map[string]any{"targetIds": ids, "sessionsRemoved": removed}, now)
	})
	if err != nil {
		return nil, 0, err
	}
	return admins, removed, audit.prune(ctx, db, now)
}

func deleteUserSessions(ctx context.Context, run database.Runner, userID string) (int64, error) {
	res, err := run.ExecContext(ctx, "DELETE FROM AuthSession WHERE userId = ?", userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// sweepSessions は期限の切れたセッションを主キーで消す（期限の範囲で DELETE して索引の隙間をロックしない）。
func sweepSessions(ctx context.Context, db *sql.DB, now time.Time) error {
	rows, err := db.QueryContext(ctx, "SELECT id FROM AuthSession WHERE expiresAt <= ? LIMIT 100", now)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, "DELETE FROM AuthSession WHERE id = ? AND expiresAt <= ?", id, now); err != nil {
			return err
		}
	}
	return nil
}
