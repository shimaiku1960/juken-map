// 期限の切れた行を定期的に消す（開発基準 06 G1「目的の無くなったデータは消す」、JUK-140）。
//
// ログインのセッション（IP・ブラウザを含む）・メールのトークン・LINE 連携の途中の値は、
// その行がもう一度使われようとしたときか、同じ人が次に同じ操作をしたときにしか消えず、
// 放っておかれた行が残り続けていた。サーバーの中で1時間ごとに消す。
//
//   - 外のタイマー（infra/systemd）に載せなかったのは、入口・秘密の値・nginx の設定が要らず、
//     開発環境でも同じように動くから。台が増えて同時に動いても、消す対象が同じなので害は無い。
//   - 消す行は先に主キーで選び、主キーで消す（sweepEmailSends と同じ）。expiresAt の範囲で DELETE すると
//     索引の隙間までロックし、空に近い表では新しい行の INSERT がそのあいだ待たされる。
//   - 管理者のセッションの「使わないときの期限」（1時間）が切れた行は、上限の期限（24時間）で消える。
//     上限の期限だけで選ぶのは、expiresAt の索引で選ぶ行の範囲だけを読むため。
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

const (
	expiredCleanupInterval = time.Hour
	// 1回の DELETE で消す行の上限。一度に多くの行をロックしない。
	expiredCleanupBatch = 500
)

// expiringTable は expiresAt（索引あり）を過ぎたら要らなくなる表。
type expiringTable struct {
	name      string
	key       string // 主キーの列
	binaryKey bool   // 主キーが BINARY（トークンのハッシュ）。文字列の列に []byte を渡すと索引が効かないので分ける
}

// expiredTables は expiresAt を持つ表のすべて。
// 2段階認証の途中（AuthMfaChallenge）と Google / GitHub ログインの途中（AuthOAuthState）は、
// 次のログインのたびにも消しているが、ログインが無い間は残るので一緒に消す。
var expiredTables = []expiringTable{
	{name: "AuthSession", key: "id"},
	{name: "AuthToken", key: "tokenHash", binaryKey: true},
	{name: "AuthMfaChallenge", key: "tokenHash", binaryKey: true},
	{name: "AuthOAuthState", key: "stateHash", binaryKey: true},
	{name: "LineLinkNonce", key: "nonce"},
	{name: "LineOAuthAttempt", key: "state"},
}

// deleteExpired は expiresAt が now 以前の行を、batch 行ずつ消す。表ごとに消した行数を返す。
func deleteExpired(ctx context.Context, db *sql.DB, now time.Time, batch int) (map[string]int64, error) {
	removed := make(map[string]int64, len(expiredTables))
	for _, table := range expiredTables {
		for {
			keys, err := expiredKeys(ctx, db, table, now, batch)
			if err != nil {
				return removed, fmt.Errorf("select expired %s: %w", table.name, err)
			}
			if len(keys) == 0 {
				break
			}
			// 条件は主キーだけにする。expiresAt も条件に入れると、小さい表では MySQL が expiresAt の索引を選び、
			// 範囲でロックしてしまう（手元の EXPLAIN で確かめた）。expiresAt を後から延ばすコードは無いので、
			// 選んだ行は消すまでのあいだも期限切れのまま。
			// #nosec G202 -- 表名と列名は expiredTables に書いた固定の名前、ほかは件数ぶん並べた ? だけ。値は keys で渡す
			res, err := db.ExecContext(ctx,
				"DELETE FROM `"+table.name+"` WHERE `"+table.key+"` IN ("+database.Placeholders(len(keys), "?")+")", keys...)
			if err != nil {
				return removed, fmt.Errorf("delete expired %s: %w", table.name, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return removed, fmt.Errorf("delete expired %s: %w", table.name, err)
			}
			removed[table.name] += n
			if len(keys) < batch {
				break
			}
		}
	}
	return removed, nil
}

// expiredKeys は期限の切れた行の主キーを batch 個まで返す（ロックしない読み取り）。
func expiredKeys(ctx context.Context, db *sql.DB, table expiringTable, now time.Time, batch int) ([]any, error) {
	// #nosec G202 -- 表名と列名は expiredTables に書いた固定の名前だけ。値は ? で渡す
	rows, err := db.QueryContext(ctx,
		"SELECT `"+table.key+"` FROM `"+table.name+"` WHERE expiresAt <= ? LIMIT ?", now, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []any
	for rows.Next() {
		if table.binaryKey {
			var key []byte
			if err := rows.Scan(&key); err != nil {
				return nil, err
			}
			keys = append(keys, key)
		} else {
			var key string
			if err := rows.Scan(&key); err != nil {
				return nil, err
			}
			keys = append(keys, key)
		}
	}
	return keys, rows.Err()
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
			attrs := make([]any, 0, len(expiredTables)*2)
			for _, table := range expiredTables {
				attrs = append(attrs, table.name, removed[table.name])
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
