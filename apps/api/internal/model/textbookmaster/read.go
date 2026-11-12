package textbookmaster

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// listLimit は一覧の上限。件数が少ないので、ページに分けず先頭から返す。
const listLimit = 200

// List は管理画面の参考書マスターの一覧（id 順、先頭200件まで）。query の q（名前・出版社・ISBN の部分一致）で絞る。
// q の形が違えば *validation.Issue を返す。
func List(ctx context.Context, db *sql.DB, query map[string][]string) ([]Master, error) {
	q, issue := validation.SearchQuery(query, "q", searchMax)
	if issue != nil {
		return nil, issue
	}
	if q == "" {
		return list(ctx, db, "")
	}
	pattern := "%" + database.EscapeLike(q) + "%"
	return list(ctx, db, "WHERE tm.name LIKE ? OR tm.publisher LIKE ? OR tm.isbn LIKE ?", pattern, pattern, pattern)
}

// list は参考書マスターを、総量の候補と利用者の参考書の数をつけて引く。where は固定の条件と ? だけ。
func list(ctx context.Context, db *sql.DB, where string, args ...any) ([]Master, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT tm.id, tm.name, tm.publisher, tm.edition, tm.isbn,
		        (SELECT COUNT(*) FROM Textbook t WHERE t.masterId = tm.id) AS textbookCount,
		        m.unit, m.totalAmount, m.isDefault
		 FROM (SELECT * FROM TextbookMaster tm `+where+` ORDER BY tm.id ASC LIMIT `+strconv.Itoa(listLimit)+`) AS tm
		 LEFT JOIN TextbookMasterMetric m ON m.masterId = tm.id
		 ORDER BY tm.id ASC, m.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	masters := []Master{}
	for rows.Next() {
		var (
			tm          Master
			unit        *string
			totalAmount *int
			isDefault   *bool
		)
		if err := rows.Scan(&tm.ID, &tm.Name, &tm.Publisher, &tm.Edition, &tm.Isbn, &tm.TextbookCount, &unit, &totalAmount, &isDefault); err != nil {
			return nil, err
		}
		// 行は（参考書 × 総量の候補）の数だけ並ぶ。同じ参考書の行は隣り合うので、直前と比べて束ねる。
		if n := len(masters); n == 0 || masters[n-1].ID != tm.ID {
			tm.Metrics = []Metric{}
			masters = append(masters, tm)
		}
		if unit != nil {
			last := &masters[len(masters)-1]
			last.Metrics = append(last.Metrics, Metric{Unit: *unit, TotalAmount: *totalAmount, IsDefault: *isDefault})
		}
	}
	return masters, rows.Err()
}
