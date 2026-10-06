package account

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// ErrUnverified は、メールアドレスを確認していない人を管理者にしようとしたこと。
var ErrUnverified = errors.New("account: メールアドレスを確認していません")

// SetRole は role を付け替え、その人のセッションをすべて消す（認証基準 10 の C4：権限の変更のたびに作り直す）。
// 前の role と、消したセッションの数を返す。セッションの期限は role で決まるので、ログインし直してもらう。
//
// メール確認前の人には admin を付けない（ErrUnverified）。他人のアドレスで登録されただけのアカウントを、
// 確認前に管理者にしてしまわないため。audit の扱いは Suspend と同じ（記録が書けなければ変えない）。
func SetRole(ctx context.Context, db *sql.DB, userID, role string, now time.Time, audit *OpsAudit) (previous string, removed int64, err error) {
	err = database.InTx(ctx, db, func(tx *sql.Tx) error {
		var verified bool
		err := tx.QueryRowContext(ctx, "SELECT emailVerified, role FROM `user` WHERE id = ? FOR UPDATE", userID).Scan(&verified, &previous)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if role == "admin" && !verified {
			return ErrUnverified
		}
		if _, err := tx.ExecContext(ctx, "UPDATE `user` SET role = ?, updatedAt = ? WHERE id = ?", role, now, userID); err != nil {
			return err
		}
		if removed, err = deleteUserSessions(ctx, tx, userID); err != nil {
			return err
		}
		return audit.insert(ctx, tx, "set-role", userID, map[string]any{
			"before": map[string]any{"role": previous}, "after": map[string]any{"role": role}, "sessionsRemoved": removed}, now)
	})
	if err != nil {
		return "", 0, err
	}
	return previous, removed, audit.prune(ctx, db, now)
}
