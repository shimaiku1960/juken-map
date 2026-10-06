package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// OpsAuditRetention は OpsAuditLog を残す期間。CloudTrail（S3 に1年）とそろえ、突き合わせられる期間を同じにする。
const OpsAuditRetention = 365 * 24 * time.Hour

// OpsAudit は、運用のコマンド（incident.go・JUK-138、セキュリティ基準 H4）から操作したときに OpsAuditLog に残す記録。
// 本番では `docker exec` で動き、出力が実行した人の端末にしか出ないので、DB に残す。
// 管理画面からの操作は nil を渡す（管理画面はログに残す。admin_users.go の logAdminUserAction）。
type OpsAudit struct {
	// Host は実行した場所（本番ではコンテナ ID）。
	Host string
}

// insert は記録を1行書く。audit が nil（管理画面からの操作）なら何もしない。
func (a *OpsAudit) insert(ctx context.Context, run database.Runner, action, targetID string, detail map[string]any, now time.Time) error {
	if a == nil {
		return nil
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	var target any // 全員が対象の操作（revoke-all など）は NULL
	if targetID != "" {
		target = targetID
	}
	if _, err := run.ExecContext(ctx,
		"INSERT INTO OpsAuditLog (action, targetId, detail, host, createdAt) VALUES (?, ?, ?, ?, ?)",
		action, target, string(raw), a.Host, now.UTC()); err != nil {
		return fmt.Errorf("記録（OpsAuditLog）に書けませんでした: %w", err)
	}
	return nil
}

// prune は残す期間を過ぎた記録を消す。書いたついでに行う（運用のコマンドはめったに使わないので、行は少ない）。
// 操作と記録を確定させた後なので、失敗しても操作は取り消さない。
func (a *OpsAudit) prune(ctx context.Context, db *sql.DB, now time.Time) error {
	if a == nil {
		return nil
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM OpsAuditLog WHERE createdAt < ?", now.UTC().Add(-OpsAuditRetention)); err != nil {
		return fmt.Errorf("操作と記録はしましたが、1年より古い記録を消せませんでした: %w", err)
	}
	return nil
}
