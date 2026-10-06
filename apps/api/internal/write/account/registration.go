package account

import (
	"context"
	"database/sql"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// メール＋パスワードでの登録と、メールアドレスの確認。

// CreateUserWithPassword は、まだ確認していない利用者とパスワードを1つのトランザクションで作る。
// hash はパスワードのハッシュ（ハッシュの作り方は入口の internal/feature/auth/password.go）。
func CreateUserWithPassword(ctx context.Context, db *sql.DB, id, email, hash string, now time.Time) error {
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO `user` (id, name, email, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, false, ?, ?)",
			id, email, email, now, now); err != nil {
			return err
		}
		return setPassword(ctx, tx, id, hash, now)
	})
}

// MarkEmailVerified はメールアドレスを確認済みにする。初めて確認済みにしたときだけ true。
func MarkEmailVerified(ctx context.Context, db *sql.DB, userID string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, "UPDATE `user` SET emailVerified = true, updatedAt = ? WHERE id = ? AND emailVerified = false", now, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}
