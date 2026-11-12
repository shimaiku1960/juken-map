// Package textbook は利用者の参考書（Textbook）のモデル。入力の決まり・読み取り・書き込み（登録と逆算設定の変更）を持ち、
// HTTP を知らない。線引きは docs/architecture.md「モデル・コントローラー・ビュー」（JUK-219・JUK-223）、
// 持ち主の決まりは同じ文書の「持ち主の一覧」（JUK-148・JUK-150）。
package textbook

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 操作が断る理由。Error() は利用者に見せる文言（コントローラーが 404・409・400 に直す）。入力の誤りは別に *validation.Issue で返す。
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
// 項目の名前・型・並びを画面に返す形（apischema の TextbookRow）とそろえ、ビューが型の変換だけで返せるようにしている。
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

// row は INSERT する値。
type row struct {
	name                  string
	masterID, totalAmount *int64
	rangeUnit, subject    *string
}

// Create は本文（createTextbookSchema）を確かめて参考書を登録し、登録した行を返す。
// 名前で作るときに同じ名前がもうあれば ErrDuplicate。
// 参考書マスターから作るときは、名前はマスターのもの、総量と単位はマスターの既定（isDefault）の候補で、
// 既定が無ければ先頭（id が最小）の候補を使う。
func Create(ctx context.Context, db *sql.DB, userID string, body any, now time.Time) (Textbook, error) {
	in, err := decodeCreate(body)
	if err != nil {
		return Textbook{}, err
	}
	r := row{name: in.name, rangeUnit: in.rangeUnit, subject: in.subject}
	if in.fromMaster {
		if r, err = fromMaster(ctx, db, in.masterID); err != nil {
			return Textbook{}, err
		}
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

// UpdateProgress は本文（updateTextbookProgressSchema）を確かめ、userID の人の参考書の逆算設定のうち、
// 送られた項目だけを書き換え、書き換えた後の行を返す。入力チェックは自分の参考書かを確かめるより先。
// 更新日時は何も送られていなくても書き換える。
func UpdateProgress(ctx context.Context, db *sql.DB, userID string, id int64, body any, now time.Time) (Textbook, error) {
	p, err := decodeProgress(body)
	if err != nil {
		return Textbook{}, err
	}
	var updated Textbook
	err = database.InTx(ctx, db, func(tx *sql.Tx) error {
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
		set("totalAmount", p.totalAmount.Present, p.totalAmount.Value)
		set("rangeUnit", p.rangeUnit.Present, p.rangeUnit.Value)
		set("targetDate", p.targetDate.Present, p.targetDate.Value)
		set("subject", p.subject.Present, p.subject.Value)
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
	t, err := scan(q.QueryRowContext(ctx,
		"SELECT "+columns+" FROM Textbook WHERE id = ? AND userId = ? LIMIT 1"+suffix, id, userID,
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Textbook{}, ErrNotFound
	}
	if err != nil {
		return Textbook{}, err
	}
	return t, nil
}
