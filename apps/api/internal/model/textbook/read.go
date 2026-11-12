package textbook

import (
	"context"
	"database/sql"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 参考書の読み取り（JUK-73）。自分の参考書の一覧と、登録するときに選ぶ参考書マスターの一覧。
// 日時は Date を JSON にしたときと同じ ISO 文字列。

// Master は参考書マスター。Metrics は総量の候補（id 順）で、無くても空のスライス（nil ではない）。
type Master struct {
	CreatedAt string
	Edition   *string
	ID        int64
	ISBN      string
	Metrics   []Metric
	Name      string
	Publisher *string
	UpdatedAt string
}

// Metric は参考書マスターの総量の候補。
type Metric struct {
	CreatedAt   string
	ID          int64
	IsDefault   bool
	MasterID    int64
	TotalAmount int64
	Unit        string
	UpdatedAt   string
}

// columns は Textbook の列と並び。scan で読む。
const columns = "id, userId, masterId, name, totalAmount, rangeUnit, targetDate, subject, createdAt, updatedAt"

// scan は columns の1行を読み、日時を ISO にする。scanFn は rows.Scan か row.Scan。
func scan(scanFn func(...any) error) (Textbook, error) {
	var t Textbook
	if err := scanFn(
		&t.ID, &t.UserID, &t.MasterID, &t.Name, &t.TotalAmount, &t.RangeUnit,
		&t.TargetDate, &t.Subject, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return t, err
	}
	if t.TargetDate != nil {
		iso := database.ISOFromDatetime(*t.TargetDate)
		t.TargetDate = &iso
	}
	t.CreatedAt = database.ISOFromDatetime(t.CreatedAt)
	t.UpdatedAt = database.ISOFromDatetime(t.UpdatedAt)
	return t, nil
}

// List は自分の参考書の一覧。名前は (userId, name) で UNIQUE なので、名前順だけで並びが決まる。
func List(ctx context.Context, db *sql.DB, userID string) ([]Textbook, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT "+columns+" FROM Textbook WHERE userId = ? ORDER BY name ASC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	textbooks := make([]Textbook, 0)
	for rows.Next() {
		t, err := scan(rows.Scan)
		if err != nil {
			return nil, err
		}
		textbooks = append(textbooks, t)
	}
	return textbooks, rows.Err()
}

// ListMasters は参考書マスターの一覧を、総量の候補（metrics）と一緒に返す。全員に同じもの。
// マスター → 総量の候補は1対多なので、LEFT JOIN 1本で取り、マスターごとに束ねる。
// 候補は id 順（登録時に「isDefault の候補、無ければ先頭」を使うので、先頭を決めておく）。
func ListMasters(ctx context.Context, db *sql.DB) ([]Master, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT tm.id, tm.name, tm.publisher, tm.edition, tm.isbn, tm.createdAt, tm.updatedAt,
		        m.id AS m_id, m.unit AS m_unit, m.totalAmount AS m_totalAmount,
		        m.isDefault AS m_isDefault, m.createdAt AS m_createdAt, m.updatedAt AS m_updatedAt
		 FROM TextbookMaster AS tm
		 LEFT JOIN TextbookMasterMetric AS m ON m.masterId = tm.id
		 ORDER BY tm.id ASC, m.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	masters := make([]Master, 0)
	for rows.Next() {
		var (
			tm Master
			// LEFT JOIN の相手が居なければ全部 NULL になる
			mID          *int64
			mUnit        *string
			mTotalAmount *int64
			mIsDefault   *bool
			mCreatedAt   *string
			mUpdatedAt   *string
		)
		if err := rows.Scan(
			&tm.ID, &tm.Name, &tm.Publisher, &tm.Edition, &tm.ISBN, &tm.CreatedAt, &tm.UpdatedAt,
			&mID, &mUnit, &mTotalAmount, &mIsDefault, &mCreatedAt, &mUpdatedAt,
		); err != nil {
			return nil, err
		}
		// ORDER BY で同じマスターの行が隣り合うので、直前の要素と比べるだけで束ねられる。
		if len(masters) == 0 || masters[len(masters)-1].ID != tm.ID {
			tm.CreatedAt = database.ISOFromDatetime(tm.CreatedAt)
			tm.UpdatedAt = database.ISOFromDatetime(tm.UpdatedAt)
			tm.Metrics = make([]Metric, 0)
			masters = append(masters, tm)
		}
		if mID != nil {
			last := &masters[len(masters)-1]
			last.Metrics = append(last.Metrics, Metric{
				ID:          *mID,
				MasterID:    last.ID,
				Unit:        *mUnit,
				TotalAmount: *mTotalAmount,
				IsDefault:   *mIsDefault,
				CreatedAt:   database.ISOFromDatetime(*mCreatedAt),
				UpdatedAt:   database.ISOFromDatetime(*mUpdatedAt),
			})
		}
	}
	return masters, rows.Err()
}
