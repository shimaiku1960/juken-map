// Package account は利用者のアカウント（user の行・ログインの状態・運用の記録）への書き込みの持ち主。
// 外に出すのは操作で、操作ごとに一緒に確定させることをトランザクションの中で済ませる。
// 持ち主の一覧と決まりは docs/architecture.md「バックエンドの構成」（JUK-148・JUK-150）。
package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// ErrNotFound は操作の相手の利用者がいないこと。
var ErrNotFound = errors.New("account: 利用者が見つかりません")

// Suspension は利用停止の結果。日時は Date#toISOString と同じ形。
type Suspension struct {
	// BannedAtBefore は止める前の bannedAt。止まっていなければ nil。
	BannedAtBefore *string
	// BannedAt は止めた日時。止め直したときは最初に止めた日時のまま。
	BannedAt        string
	SessionsRemoved int64
}

// Suspend は利用者を止め、その人のセッションをすべて消す。両方そろって初めて「止まった」と言える
// （次のログインは internal/feature/auth/handlers.go・internal/feature/auth/mfa.go・internal/feature/auth/oauth.go が bannedAt を見て断る）。認証基準 10 の C5 の
// 「ある利用者の全端末」にあたる。bannedAt を先に書くので、消している間に新しく入られても、そのログインは断られる。
//
// 管理画面（internal/feature/admin/users.go）と運用のコマンド（incident.go）の両方がこれを呼ぶ。audit を渡すと（運用のコマンド）、
// 同じトランザクションで OpsAuditLog に記録する。記録が書けなければ止めない。
func Suspend(ctx context.Context, db *sql.DB, userID string, now time.Time, audit *OpsAudit) (Suspension, error) {
	var s Suspension
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		before, err := lockBannedAt(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE `user` SET bannedAt = COALESCE(bannedAt, ?), updatedAt = ? WHERE id = ?", now, now, userID); err != nil {
			return err
		}
		// 書いた値を読み直す。DATETIME(3) は端数を丸めるので、渡した now ではなく DB の値を返す。
		after, err := lockBannedAt(ctx, tx, userID)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "DELETE FROM AuthSession WHERE userId = ?", userID)
		if err != nil {
			return err
		}
		removed, err := res.RowsAffected()
		if err != nil {
			return err
		}
		s = Suspension{BannedAtBefore: before, BannedAt: *after, SessionsRemoved: removed}
		if err := audit.insert(ctx, tx, "ban", userID, map[string]any{
			"before": map[string]any{"bannedAt": before}, "after": map[string]any{"bannedAt": after}, "sessionsRemoved": removed}, now); err != nil {
			return fmt.Errorf("止めていません。%w", err)
		}
		return nil
	})
	if err != nil {
		return Suspension{}, err
	}
	return s, audit.prune(ctx, db, now)
}

// Unsuspend は止めたのを戻す。止まっていなければ何も変わらない。セッションは Suspend で消したままなので、
// 戻した人はログインし直す。audit の扱いは Suspend と同じ。
func Unsuspend(ctx context.Context, db *sql.DB, userID string, now time.Time, audit *OpsAudit) error {
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		before, err := lockBannedAt(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE `user` SET bannedAt = NULL, updatedAt = ? WHERE id = ?", now, userID); err != nil {
			return err
		}
		if err := audit.insert(ctx, tx, "unban", userID, map[string]any{
			"before": map[string]any{"bannedAt": before}, "after": map[string]any{"bannedAt": nil}}, now); err != nil {
			return fmt.Errorf("停止を戻していません。%w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return audit.prune(ctx, db, now)
}

// lockBannedAt はその人の行を押さえて bannedAt を読む。止まっていなければ nil、いなければ ErrNotFound。
// 押さえるのは、同時に止めたり戻したりしたときに、記録の前後の値が実際の順番とずれないようにするため。
func lockBannedAt(ctx context.Context, tx *sql.Tx, userID string) (*string, error) {
	var banned sql.NullString
	err := tx.QueryRowContext(ctx, "SELECT bannedAt FROM `user` WHERE id = ? FOR UPDATE", userID).Scan(&banned)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil || !banned.Valid {
		return nil, err
	}
	iso := database.ISOFromDatetime(banned.String)
	return &iso, nil
}
