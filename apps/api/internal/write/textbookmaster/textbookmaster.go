// Package textbookmaster は参考書マスター（TextbookMaster と総量の候補 TextbookMasterMetric）への書き込みの持ち主。
// 管理画面のマスター編集（JUK-78）の作成・書き換え・削除を操作として出す。持ち主の一覧と決まりは
// docs/architecture.md「バックエンドの構成」（JUK-148・JUK-150）。
//
// 利用者の参考書（Textbook.masterId）は外部キーが SET NULL で黙って紐づきが外れるので、1冊でも登録されていれば消さない。
// 利用者が登録した参考書は総量を自分の行に写し取っているので、マスターの総量を変えても既存の参考書は変わらない
// （これから登録する人から効く）。
//
// 入力の形の確かめ（Zod と同じ 400）は入口が行い、ここには確かめ済みの値が来る。参考書マスターの一覧のキャッシュを捨てるのも、
// 変更をログに残すのも入口（操作が成功して確定したあと）。
package textbookmaster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 操作が断る理由。入口が利用者向けの文言（404・409）に直す。
var (
	// ErrNotFound は、参考書マスターが無いこと。
	ErrNotFound = errors.New("textbookmaster: not found")
	// ErrDuplicate は、同じ ISBN の参考書マスターがあること（DB の一意制約）。
	ErrDuplicate = errors.New("textbookmaster: duplicate isbn")
)

// InUseError は、利用者の参考書に使われているので消せないこと。Count は使っている参考書の冊数（文言に入れる）。
type InUseError struct{ Count int }

func (e *InUseError) Error() string {
	return fmt.Sprintf("textbookmaster: used by %d textbooks", e.Count)
}

// Metric は総量の候補（単位ごと）。項目の並びは apischema の AdminTextbookMasterMetric とそろえている。
type Metric struct {
	// IsDefault は、利用者が参考書を登録したときにこの候補で総量が入ること。
	IsDefault   bool
	TotalAmount int
	Unit        string
}

// Master は管理画面の参考書マスターの行。項目の並びは apischema の AdminTextbookMaster とそろえている。
type Master struct {
	Edition   *string
	ID        int64
	Isbn      string
	Metrics   []Metric
	Name      string
	Publisher *string
	// TextbookCount は、この参考書から登録された利用者の参考書の数。
	TextbookCount int
}

// Change は書き換えの前と後。入口は監査ログに両方を残し、応答は後を返す。
type Change struct {
	Before, After Master
}

// Input は参考書マスターの作成・書き換えの値。Metrics は1つ以上で、既定（IsDefault）はちょうど1つ（入口が確かめる）。
type Input struct {
	Name               string
	Publisher, Edition *string
	Isbn               string
	Metrics            []Metric
}

// Create は参考書マスターを総量の候補つきで作る。同じ ISBN があれば ErrDuplicate。
func Create(ctx context.Context, db *sql.DB, in Input, now time.Time) (Master, error) {
	var created Master
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO TextbookMaster (name, publisher, edition, isbn, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)`,
			in.Name, in.Publisher, in.Edition, in.Isbn, now, now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := replaceMetrics(ctx, tx, id, in.Metrics, now); err != nil {
			return err
		}
		created, err = find(ctx, tx, id)
		return err
	})
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return Master{}, ErrDuplicate
	}
	return created, err
}

// Update は参考書マスターと総量の候補を書き換え、前と後を返す。無ければ ErrNotFound、ISBN が重なれば ErrDuplicate。
func Update(ctx context.Context, db *sql.DB, id int64, in Input, now time.Time) (Change, error) {
	var change Change
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		var err error
		if change.Before, err = lockAndFind(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE TextbookMaster SET name = ?, publisher = ?, edition = ?, isbn = ?, updatedAt = ? WHERE id = ?",
			in.Name, in.Publisher, in.Edition, in.Isbn, now, id); err != nil {
			return err
		}
		if err := replaceMetrics(ctx, tx, id, in.Metrics, now); err != nil {
			return err
		}
		change.After, err = find(ctx, tx, id)
		return err
	})
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return Change{}, ErrDuplicate
	}
	return change, err
}

// Delete は参考書マスターを（総量の候補ごと）消し、消した行を返す。利用者の参考書に使われていれば *InUseError。
func Delete(ctx context.Context, db *sql.DB, id int64) (Master, error) {
	var deleted Master
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		// 押さえておけば、数えたあとにこのマスターから参考書が登録されることはない（登録の外部キーの確かめが待つ）。
		var err error
		if deleted, err = lockAndFind(ctx, tx, id); err != nil {
			return err
		}
		if deleted.TextbookCount > 0 {
			return &InUseError{Count: deleted.TextbookCount}
		}
		// 総量の候補は外部キーの CASCADE で一緒に消える。
		_, err = tx.ExecContext(ctx, "DELETE FROM TextbookMaster WHERE id = ?", id)
		return err
	})
	return deleted, err
}

// lockAndFind は参考書マスターの行を FOR UPDATE で押さえてから読む。無ければ ErrNotFound。
func lockAndFind(ctx context.Context, tx *sql.Tx, id int64) (Master, error) {
	var locked int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM TextbookMaster WHERE id = ? FOR UPDATE", id).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return Master{}, ErrNotFound
	}
	if err != nil {
		return Master{}, err
	}
	return find(ctx, tx, id)
}

// find は参考書マスターを、総量の候補（id の順）と利用者の参考書の数をつけて読む。無ければ ErrNotFound。
func find(ctx context.Context, tx *sql.Tx, id int64) (Master, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT tm.id, tm.name, tm.publisher, tm.edition, tm.isbn,
		        (SELECT COUNT(*) FROM Textbook t WHERE t.masterId = tm.id) AS textbookCount,
		        m.unit, m.totalAmount, m.isDefault
		 FROM TextbookMaster tm
		 LEFT JOIN TextbookMasterMetric m ON m.masterId = tm.id
		 WHERE tm.id = ?
		 ORDER BY m.id ASC`, id)
	if err != nil {
		return Master{}, err
	}
	defer rows.Close()
	var (
		master Master
		found  bool
	)
	for rows.Next() {
		var (
			unit        *string
			totalAmount *int
			isDefault   *bool
		)
		if err := rows.Scan(&master.ID, &master.Name, &master.Publisher, &master.Edition, &master.Isbn, &master.TextbookCount,
			&unit, &totalAmount, &isDefault); err != nil {
			return Master{}, err
		}
		if !found {
			found, master.Metrics = true, []Metric{}
		}
		// 総量の候補が無ければ、候補の列が NULL の行が1行だけ来る
		if unit != nil {
			master.Metrics = append(master.Metrics, Metric{IsDefault: *isDefault, TotalAmount: *totalAmount, Unit: *unit})
		}
	}
	if err := rows.Err(); err != nil {
		return Master{}, err
	}
	if !found {
		return Master{}, ErrNotFound
	}
	return master, nil
}

// replaceMetrics は総量の候補を送られたものに置き換える（入口の確かめで1つ以上ある）。
func replaceMetrics(ctx context.Context, tx *sql.Tx, masterID int64, metrics []Metric, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM TextbookMasterMetric WHERE masterId = ?", masterID); err != nil {
		return err
	}
	args := make([]any, 0, len(metrics)*6)
	for _, m := range metrics {
		args = append(args, masterID, m.Unit, m.TotalAmount, m.IsDefault, now, now)
	}
	// #nosec G202 -- 埋め込むのは件数ぶん並べた (?, …) だけ。値は args で渡す
	_, err := tx.ExecContext(ctx,
		`INSERT INTO TextbookMasterMetric (masterId, unit, totalAmount, isDefault, createdAt, updatedAt)
		 VALUES `+database.Placeholders(len(metrics), "(?, ?, ?, ?, ?, ?)"), args...)
	return err
}
