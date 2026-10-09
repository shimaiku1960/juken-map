// Package textbook は利用者の参考書（Textbook）への書き込みの持ち主。登録（名前から・参考書マスターから）と、
// 逆算設定の変更を操作として出す。持ち主の一覧と決まりは docs/architecture.md「バックエンドの構成」（JUK-148・JUK-150）。
//
// 入力の形の確かめ（Zod と同じ 400）は入口が行い、ここには確かめ済みの値が来る。
package textbook

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// 操作が断る理由。Error() は利用者に見せる文言（入口が 404・409・400 に直す）。
var (
	// ErrNotFound は、参考書が無いか、他人のものであること。
	ErrNotFound = errors.New("参考書が見つかりません")
	// ErrDuplicate は、同じ名前の参考書をもう登録していること（DB の一意制約 userId, name）。
	ErrDuplicate = errors.New("この参考書はすでに登録されています")
	// ErrMasterNotFound は、指定した参考書マスターが無いこと。
	ErrMasterNotFound = errors.New("参考書マスターが見つかりません")
	// ErrMasterNoMetric は、参考書マスターに総量の候補が1つも無いこと。
	ErrMasterNoMetric = errors.New("参考書の総量データが登録されていません")
)

// Textbook は参考書の行。日時は Date#toISOString と同じ形。
// 項目の名前・型・並びを画面に返す形（apischema の TextbookRow）とそろえ、入口が型の変換だけで返せるようにしている。
type Textbook struct {
	CreatedAt   string
	ID          int64
	MasterID    *int64
	Name        string
	RangeUnit   *string
	Subject     *string
	TargetDate  *string
	TotalAmount *int64
	UpdatedAt   string
	UserID      string
}

// New は名前から登録する参考書の値。null の項目は nil。
type New struct {
	Name               string
	RangeUnit, Subject *string
}

// Progress は逆算設定の変更。Present の項目だけを書き換える。TargetDate はその日の 00:00 UTC。
type Progress struct {
	TotalAmount        opt.Field[int64]
	RangeUnit, Subject opt.Field[string]
	TargetDate         opt.Field[time.Time]
}

// row は INSERT する値。
type row struct {
	name                  string
	masterID, totalAmount *int64
	rangeUnit, subject    *string
}

// Create は名前から参考書を登録する。同じ名前がもうあれば ErrDuplicate。
func Create(ctx context.Context, db *sql.DB, userID string, in New, now time.Time) (Textbook, error) {
	return insert(ctx, db, userID, row{name: in.Name, rangeUnit: in.RangeUnit, subject: in.Subject}, now)
}

// CreateFromMaster は参考書マスターから参考書を登録する。名前はマスターのもの、
// 総量と単位はマスターの既定（isDefault）の候補で、既定が無ければ先頭（id が最小）の候補を使う。
func CreateFromMaster(ctx context.Context, db *sql.DB, userID string, masterID int64, now time.Time) (Textbook, error) {
	r, err := fromMaster(ctx, db, masterID)
	if err != nil {
		return Textbook{}, err
	}
	return insert(ctx, db, userID, r, now)
}

func insert(ctx context.Context, db *sql.DB, userID string, r row, now time.Time) (Textbook, error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO Textbook
		   (userId, name, masterId, totalAmount, rangeUnit, subject, createdAt, updatedAt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, r.name, r.masterID, r.totalAmount, r.rangeUnit, r.subject, now, now)
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return Textbook{}, ErrDuplicate
	}
	if err != nil {
		return Textbook{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Textbook{}, err
	}
	// INSERT は行を返さないので、応答に使う形を読み直す。
	return find(ctx, db, id, userID, "")
}

// fromMaster は参考書マスターから登録する値を作る。
func fromMaster(ctx context.Context, db *sql.DB, masterID int64) (row, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT tm.name, m.unit, m.totalAmount, m.isDefault
		 FROM TextbookMaster AS tm
		 LEFT JOIN TextbookMasterMetric AS m ON m.masterId = tm.id
		 WHERE tm.id = ?
		 ORDER BY m.id ASC`,
		masterID)
	if err != nil {
		return row{}, err
	}
	defer rows.Close()

	var (
		name   string
		found  bool
		first  *row
		chosen *row
	)
	for rows.Next() {
		var (
			unit        *string
			totalAmount *int64
			isDefault   *bool
		)
		if err := rows.Scan(&name, &unit, &totalAmount, &isDefault); err != nil {
			return row{}, err
		}
		found = true
		// LEFT JOIN の相手（総量の候補）が居なければ、候補の列が NULL の行が1行だけ来る
		if unit == nil {
			continue
		}
		candidate := &row{masterID: &masterID, totalAmount: totalAmount, rangeUnit: unit}
		if first == nil {
			first = candidate
		}
		if chosen == nil && *isDefault {
			chosen = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return row{}, err
	}
	switch {
	case !found:
		return row{}, ErrMasterNotFound
	case chosen == nil && first == nil:
		return row{}, ErrMasterNoMetric
	case chosen == nil:
		chosen = first
	}
	chosen.name = name
	return *chosen, nil
}

// UpdateProgress は userID の人の参考書の逆算設定のうち、送られた項目だけを書き換え、書き換えた後の行を返す。
// 更新日時は何も送られていなくても書き換える。
func UpdateProgress(ctx context.Context, db *sql.DB, userID string, id int64, p Progress, now time.Time) (Textbook, error) {
	var updated Textbook
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := find(ctx, tx, id, userID, " FOR UPDATE"); err != nil {
			return err
		}
		var sets []string
		var args []any
		set := func(column string, present bool, value any) {
			if present {
				sets = append(sets, column+" = ?")
				args = append(args, value)
			}
		}
		set("totalAmount", p.TotalAmount.Present, p.TotalAmount.Value)
		set("rangeUnit", p.RangeUnit.Present, p.RangeUnit.Value)
		set("targetDate", p.TargetDate.Present, p.TargetDate.Value)
		set("subject", p.Subject.Present, p.Subject.Value)
		set("updatedAt", true, now)

		// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（set の1つ目）。値は args で ? として渡す
		if _, err := tx.ExecContext(ctx,
			"UPDATE Textbook SET "+strings.Join(sets, ", ")+" WHERE id = ? AND userId = ?",
			append(args, id, userID)...); err != nil {
			return err
		}
		var err error
		updated, err = find(ctx, tx, id, userID, "")
		return err
	})
	return updated, err
}

// find は userID の人の参考書を1件読む。無いか他人のものなら ErrNotFound。
// suffix には、変える前に行を押さえる " FOR UPDATE" を渡せる。
func find(ctx context.Context, q database.QueryRower, id int64, userID, suffix string) (Textbook, error) {
	var t Textbook
	err := q.QueryRowContext(ctx,
		`SELECT id, userId, masterId, name, totalAmount, rangeUnit, targetDate, subject, createdAt, updatedAt
		 FROM Textbook WHERE id = ? AND userId = ? LIMIT 1`+suffix, id, userID,
	).Scan(&t.ID, &t.UserID, &t.MasterID, &t.Name, &t.TotalAmount, &t.RangeUnit,
		&t.TargetDate, &t.Subject, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Textbook{}, ErrNotFound
	}
	if err != nil {
		return Textbook{}, err
	}
	if t.TargetDate != nil {
		iso := database.ISOFromDatetime(*t.TargetDate)
		t.TargetDate = &iso
	}
	t.CreatedAt = database.ISOFromDatetime(t.CreatedAt)
	t.UpdatedAt = database.ISOFromDatetime(t.UpdatedAt)
	return t, nil
}
