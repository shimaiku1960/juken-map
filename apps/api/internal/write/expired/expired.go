package expired

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// Table は expiresAt（索引あり）を過ぎたら要らなくなる表。どの表を消すかは、表の持ち主
// （internal/write/...）が決めて Delete を呼ぶ。消し方（下）はどの表も同じなので、持ち主の共通の部品としてここに置く。
type Table struct {
	Name      string
	Key       string // 主キーの列
	BinaryKey bool   // 主キーが BINARY（トークンのハッシュ）。文字列の列に []byte を渡すと索引が効かないので分ける
}

// Delete は tables の expiresAt が now 以前の行を、batch 行ずつ消す。表ごとに消した行数を返す。
//
// 消す行は先に主キーで選び、主キーで消す。expiresAt の範囲で DELETE すると索引の隙間までロックし、
// 空に近い表では新しい行の INSERT がそのあいだ待たされる。
func Delete(ctx context.Context, db *sql.DB, tables []Table, now time.Time, batch int) (map[string]int64, error) {
	removed := make(map[string]int64, len(tables))
	for _, table := range tables {
		for {
			keys, err := expiredKeys(ctx, db, table, now, batch)
			if err != nil {
				return removed, fmt.Errorf("select expired %s: %w", table.Name, err)
			}
			if len(keys) == 0 {
				break
			}
			// 条件は主キーだけにする。expiresAt も条件に入れると、小さい表では MySQL が expiresAt の索引を選び、
			// 範囲でロックしてしまう（手元の EXPLAIN で確かめた）。expiresAt を後から延ばすコードは無いので、
			// 選んだ行は消すまでのあいだも期限切れのまま。
			// #nosec G202 -- 表名と列名は持ち主が書いた固定の名前、ほかは件数ぶん並べた ? だけ。値は keys で渡す
			res, err := db.ExecContext(ctx,
				"DELETE FROM `"+table.Name+"` WHERE `"+table.Key+"` IN ("+database.Placeholders(len(keys), "?")+")", keys...)
			if err != nil {
				return removed, fmt.Errorf("delete expired %s: %w", table.Name, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return removed, fmt.Errorf("delete expired %s: %w", table.Name, err)
			}
			removed[table.Name] += n
			if len(keys) < batch {
				break
			}
		}
	}
	return removed, nil
}

// expiredKeys は期限の切れた行の主キーを batch 個まで返す（ロックしない読み取り）。
func expiredKeys(ctx context.Context, db *sql.DB, table Table, now time.Time, batch int) ([]any, error) {
	// #nosec G202 -- 表名と列名は持ち主が書いた固定の名前だけ。値は ? で渡す
	rows, err := db.QueryContext(ctx,
		"SELECT `"+table.Key+"` FROM `"+table.Name+"` WHERE expiresAt <= ? LIMIT ?", now, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []any
	for rows.Next() {
		if table.BinaryKey {
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
