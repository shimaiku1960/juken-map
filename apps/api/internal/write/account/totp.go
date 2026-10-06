package account

import (
	"context"
	"database/sql"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 2段階認証の TOTP と予備コード（認証基準 10 の G1・G2）。秘密の暗号化・コードの確かめ方・予備コードの
// ハッシュの作り方は入口（auth_totp.go）で、ここには暗号化した秘密とハッシュだけが来る。

// StartTOTPSetup は暗号化した秘密を、まだ有効にしていない状態で保存し、予備コードを作り直す。
// 作り直したら古い予備コードは全部無効になる（G2）。
func StartTOTPSetup(ctx context.Context, db *sql.DB, userID, sealed string, backupHashes [][]byte, now time.Time) error {
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO AuthTotp (userId, secret, createdAt, enabledAt, lastUsedStep) VALUES (?, ?, ?, NULL, NULL)
			 ON DUPLICATE KEY UPDATE secret = VALUES(secret), createdAt = VALUES(createdAt), enabledAt = NULL, lastUsedStep = NULL`,
			userID, sealed, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM AuthBackupCode WHERE userId = ?", userID); err != nil {
			return err
		}
		for _, hash := range backupHashes {
			if _, err := tx.ExecContext(ctx, "INSERT INTO AuthBackupCode (userId, codeHash) VALUES (?, ?)", userID, hash); err != nil {
				return err
			}
		}
		return nil
	})
}

// EnableTOTP は設定の途中の TOTP を有効にし、確かめたステップを使用済みにする。途中のものが無ければ false。
func EnableTOTP(ctx context.Context, db *sql.DB, userID string, step int64, now time.Time) (bool, error) {
	return updatedOne(db.ExecContext(ctx, "UPDATE AuthTotp SET enabledAt = ?, lastUsedStep = ? WHERE userId = ? AND enabledAt IS NULL", now, step, userID))
}

// MarkTOTPStepUsed はステップを使用済みにする。同じステップ以前がもう使われていれば false（同じコードを2回使わせない）。
func MarkTOTPStepUsed(ctx context.Context, db *sql.DB, userID string, step int64) (bool, error) {
	return updatedOne(db.ExecContext(ctx,
		"UPDATE AuthTotp SET lastUsedStep = ? WHERE userId = ? AND (lastUsedStep IS NULL OR lastUsedStep < ?)", step, userID, step))
}

// ResealTOTP は秘密を新しい鍵で暗号化し直したものに置き換える。
func ResealTOTP(ctx context.Context, db *sql.DB, userID, sealed string) error {
	_, err := db.ExecContext(ctx, "UPDATE AuthTotp SET secret = ? WHERE userId = ?", sealed, userID)
	return err
}

// UseBackupCode は予備コードを消す（1回だけ使える。G2）。そのハッシュの予備コードがあれば true。
func UseBackupCode(ctx context.Context, db *sql.DB, userID string, codeHash []byte) (bool, error) {
	return updatedOne(db.ExecContext(ctx, "DELETE FROM AuthBackupCode WHERE userId = ? AND codeHash = ?", userID, codeHash))
}

// ResetTwoFactor は2段階認証を設定する前に戻し（TOTP・予備コード・途中の状態を消す）、セッションをすべて消す。
// 消したセッションの数を返す。audit の扱いは Suspend と同じ（記録が書けなければ戻さない）。
func ResetTwoFactor(ctx context.Context, db *sql.DB, userID string, now time.Time, audit *OpsAudit) (int64, error) {
	var removed int64
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM AuthTotp WHERE userId = ?",
			"DELETE FROM AuthBackupCode WHERE userId = ?",
			"DELETE FROM AuthMfaChallenge WHERE userId = ?",
		} {
			if _, err := tx.ExecContext(ctx, q, userID); err != nil {
				return err
			}
		}
		var err error
		if removed, err = deleteUserSessions(ctx, tx, userID); err != nil {
			return err
		}
		return audit.insert(ctx, tx, "reset-2fa", userID, map[string]any{"sessionsRemoved": removed}, now)
	})
	if err != nil {
		return 0, err
	}
	return removed, audit.prune(ctx, db, now)
}

// updatedOne は、1行だけ変えた（消した）ときに true を返す。
func updatedOne(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
