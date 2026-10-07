package authguard

import (
	"context"
	"database/sql"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/expired"
)

// expiringTables は authguard の表のうち、期限（expiresAt）を過ぎたら要らなくなるもの。
// Google / GitHub ログインの途中（AuthOAuthState）は次のログインのたびにも消しているが、ログインが無い間は残る。
// 回数の制限（AuthThrottle）とメールの送信（EmailSend）は expiresAt を持たず、使うたびに古いものを消す。
var expiringTables = []expired.Table{
	{Name: "AuthOAuthState", Key: "stateHash", BinaryKey: true},
}

// DeleteExpired は期限の切れた行を batch 行ずつ消し、表ごとに消した行数を返す（定期的な掃除。internal/app の expired_cleanup.go）。
func DeleteExpired(ctx context.Context, db *sql.DB, now time.Time, batch int) (map[string]int64, error) {
	return expired.Delete(ctx, db, expiringTables, now, batch)
}
