package account

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// パスワード（06 B6・10 E3）、メールで送るトークン（10 E1）、2段階認証の途中の状態（G3）。
// トークンを作る・Cookie に入れるのは入口で、ここにはトークンの SHA-256 だけが来る。

// SetPassword はパスワードのハッシュを保存する（無ければ作る）。外部ログインだけの人が初めて設定するときにも使う。
func SetPassword(ctx context.Context, db *sql.DB, userID, hash string, now time.Time) error {
	return setPassword(ctx, db, userID, hash, now)
}

func setPassword(ctx context.Context, run database.Runner, userID, hash string, now time.Time) error {
	_, err := run.ExecContext(ctx,
		"INSERT INTO AuthPassword (userId, hash, updatedAt) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE hash = VALUES(hash), updatedAt = VALUES(updatedAt)",
		userID, hash, now)
	return err
}

// ReplacePassword はパスワードを置き換え、keepSessionID 以外のセッションと、まだ使われていない
// 再設定・確認のトークン、2段階認証の途中の状態を消す（06 B6・10 E3）。keepSessionID が空なら全セッションを消す。
func ReplacePassword(ctx context.Context, db *sql.DB, userID, hash, keepSessionID string, now time.Time) error {
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		if err := setPassword(ctx, tx, userID, hash, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM AuthSession WHERE userId = ? AND id <> ?", userID, keepSessionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM AuthToken WHERE userId = ?", userID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM AuthMfaChallenge WHERE userId = ?", userID)
		return err
	})
}

// IssueToken は用途つきのトークンを保存する。同じ人・同じ用途の古いものと、期限の切れたものは消す。
func IssueToken(ctx context.Context, db *sql.DB, userID, purpose string, tokenHash []byte, ttl time.Duration, now time.Time) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM AuthToken WHERE userId = ? AND (purpose = ? OR expiresAt <= ?)", userID, purpose, now); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx,
		"INSERT INTO AuthToken (tokenHash, purpose, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?, ?)",
		tokenHash, purpose, userID, now, now.Add(ttl))
	return err
}

// ConsumeToken はトークンを使い、持ち主を返す。用途が違う・期限切れ・使用済みなら空。
// 消せたときだけ使えたことにするので、同じトークンが同時に2回送られても1回しか通らない。
func ConsumeToken(ctx context.Context, db *sql.DB, tokenHash []byte, purpose string, now time.Time) (string, error) {
	var userID string
	err := db.QueryRowContext(ctx,
		"SELECT userId FROM AuthToken WHERE tokenHash = ? AND purpose = ? AND expiresAt > ?", tokenHash, purpose, now).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	res, err := db.ExecContext(ctx,
		"DELETE FROM AuthToken WHERE tokenHash = ? AND purpose = ? AND expiresAt > ?", tokenHash, purpose, now)
	if err != nil {
		return "", err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return "", err
	}
	return userID, nil
}

// CreateMFAChallenge は2段階認証の途中の状態を作る。同じ人の前のものと、期限の切れたものは消す。
func CreateMFAChallenge(ctx context.Context, db *sql.DB, userID string, tokenHash []byte, ttl time.Duration, now time.Time) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM AuthMfaChallenge WHERE userId = ? OR expiresAt <= ?", userID, now); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx,
		"INSERT INTO AuthMfaChallenge (tokenHash, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?)",
		tokenHash, userID, now, now.Add(ttl))
	return err
}

// CountMFAChallengeAttempt は途中の状態1つで試した数を1つ増やし、持ち主を返す（H1）。期限切れ・試行が
// maxAttempts に達した・無いなら空。数えてから確かめるので、同時に送られても上限を超えない。
func CountMFAChallengeAttempt(ctx context.Context, db *sql.DB, tokenHash []byte, maxAttempts int, now time.Time) (string, error) {
	res, err := db.ExecContext(ctx,
		"UPDATE AuthMfaChallenge SET attempts = attempts + 1 WHERE tokenHash = ? AND expiresAt > ? AND attempts < ?",
		tokenHash, now, maxAttempts)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return "", nil
	}
	var userID string
	if err := db.QueryRowContext(ctx, "SELECT userId FROM AuthMfaChallenge WHERE tokenHash = ?", tokenHash).Scan(&userID); err != nil {
		return "", err
	}
	return userID, nil
}

// DeleteMFAChallenges はその人の途中の状態を消す（2段階認証を通ったとき）。
func DeleteMFAChallenges(ctx context.Context, db *sql.DB, userID string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM AuthMfaChallenge WHERE userId = ?", userID)
	return err
}
