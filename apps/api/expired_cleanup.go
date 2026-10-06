// 期限の切れた行を定期的に消す（開発基準 06 G1「目的の無くなったデータは消す」、JUK-140）。
//
// ログインのセッション（IP・ブラウザを含む）・メールのトークン・LINE 連携の途中の値は、
// その行がもう一度使われようとしたときか、同じ人が次に同じ操作をしたときにしか消えず、
// 放っておかれた行が残り続けていた。サーバーの中で1時間ごとに消す。
//
//   - 外のタイマー（infra/systemd）に載せなかったのは、入口・秘密の値・nginx の設定が要らず、
//     開発環境でも同じように動くから。台が増えて同時に動いても、消す対象が同じなので害は無い。
//   - どの表を消すかは表の持ち主（internal/write/account・authguard・notification）が決め、それぞれの
//     DeleteExpired を順に呼ぶ（JUK-154）。表ごとに別々に消してよく、まとめて確定させる必要は無い。
//     消し方（主キーで選んで主キーで消す）は internal/database の DeleteExpired。
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/authguard"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/notification"
)

const (
	expiredCleanupInterval = time.Hour
	// 1回の DELETE で消す行の上限。一度に多くの行をロックしない。
	expiredCleanupBatch = 500
)

// expiredCleanups は期限の切れた行を消す、持ち主ごとの操作。
var expiredCleanups = []func(ctx context.Context, db *sql.DB, now time.Time, batch int) (map[string]int64, error){
	account.DeleteExpired,
	authguard.DeleteExpired,
	notification.DeleteExpired,
}

// deleteExpired は持ち主ごとの操作を順に呼び、表ごとに消した行数をまとめて返す。
func deleteExpired(ctx context.Context, db *sql.DB, now time.Time, batch int) (map[string]int64, error) {
	removed := map[string]int64{}
	for _, cleanup := range expiredCleanups {
		got, err := cleanup(ctx, db, now, batch)
		maps.Copy(removed, got)
		if err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// runExpiredCleanup は起動したときと、その後1時間ごとに期限の切れた行を消す。ctx が取り消されたら戻る。
// 消せなかったときはログに出して、次の回にまた試す。
func runExpiredCleanup(ctx context.Context, db *sql.DB, now func() time.Time) {
	ticker := time.NewTicker(expiredCleanupInterval)
	defer ticker.Stop()
	for {
		removed, err := deleteExpired(ctx, db, now().UTC(), expiredCleanupBatch)
		switch {
		case err != nil && ctx.Err() == nil:
			slog.Error("[expired-cleanup] Failed to delete expired rows.", "err", err.Error())
		case err == nil:
			attrs := make([]any, 0, len(removed)*2)
			for _, table := range slices.Sorted(maps.Keys(removed)) {
				attrs = append(attrs, table, removed[table])
			}
			slog.Info("[expired-cleanup] Deleted expired rows.", attrs...)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
