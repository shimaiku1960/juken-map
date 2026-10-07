package account

import (
	"context"
	"database/sql"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/expired"
)

// expiringTables は account の表のうち、期限（expiresAt）を過ぎたら要らなくなるもの。
// 2段階認証の途中（AuthMfaChallenge）は次のログインのたびにも消しているが、ログインが無い間は残るので一緒に消す。
// 管理者のセッションの「使わないときの期限」（1時間）が切れた行は、上限の期限（24時間）で消える。
// 上限の期限だけで選ぶのは、expiresAt の索引で選ぶ行の範囲だけを読むため。
var expiringTables = []expired.Table{
	{Name: "AuthSession", Key: "id"},
	{Name: "AuthToken", Key: "tokenHash", BinaryKey: true},
	{Name: "AuthMfaChallenge", Key: "tokenHash", BinaryKey: true},
}

// DeleteExpired は期限の切れた行を batch 行ずつ消し、表ごとに消した行数を返す（定期的な掃除。internal/app の expired_cleanup.go）。
func DeleteExpired(ctx context.Context, db *sql.DB, now time.Time, batch int) (map[string]int64, error) {
	return expired.Delete(ctx, db, expiringTables, now, batch)
}
