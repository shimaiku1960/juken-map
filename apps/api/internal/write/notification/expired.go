package notification

import (
	"context"
	"database/sql"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// expiringTables は notification の表のうち、期限（expiresAt）を過ぎたら要らなくなるもの（LINE 連携の途中の値）。
var expiringTables = []database.ExpiringTable{
	{Name: "LineLinkNonce", Key: "nonce"},
	{Name: "LineOAuthAttempt", Key: "state"},
}

// DeleteExpired は期限の切れた行を batch 行ずつ消し、表ごとに消した行数を返す（定期的な掃除。expired_cleanup.go）。
func DeleteExpired(ctx context.Context, db *sql.DB, now time.Time, batch int) (map[string]int64, error) {
	return database.DeleteExpired(ctx, db, expiringTables, now, batch)
}
