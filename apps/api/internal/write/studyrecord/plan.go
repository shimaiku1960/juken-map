package studyrecord

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// PlanItem は予定1件の中身。
type PlanItem struct {
	TextbookID, RangeStart, RangeEnd *int64
	RangeUnit, Content, Subject      *string
}

// PlanPatch は予定の変更。Present の項目だけを書き換える。Date はその日の 00:00 UTC。
type PlanPatch struct {
	Date                             opt.Field[time.Time]
	TextbookID, RangeStart, RangeEnd opt.Field[int64]
	RangeUnit, Content, Subject      opt.Field[string]
	Done                             opt.Field[bool]
}

// Completion は予定の完了で書く実績の値。範囲は、送られていなければ（キーが無ければ）予定の値を使う。
type Completion struct {
	Minutes              int64
	RangeStart, RangeEnd opt.Field[int64]
	RangeUnit            opt.Field[string]
	Memo                 *string
}

// Completed は完了にした予定と、作った実績。
type Completed struct {
	Log             Log
	Plan            Plan
	IsFirstStudyLog bool
}

const planColumns = `
  p.id, p.userId, p.date, p.content, p.subject, p.done, p.textbookId,
  p.rangeStart, p.rangeEnd, p.rangeUnit, p.createdAt, p.updatedAt`

// scanPlan は planColumns の後ろに extra の列を続けて読む。rawDate は書き戻せるよう DB の文字列のまま返す。
func scanPlan(row *sql.Row, extra ...any) (Plan, string, error) {
	var p Plan
	var rawDate string
	err := row.Scan(append([]any{&p.ID, &p.UserID, &rawDate, &p.Content, &p.Subject, &p.Done, &p.TextbookID,
		&p.RangeStart, &p.RangeEnd, &p.RangeUnit, &p.CreatedAt, &p.UpdatedAt}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, "", ErrNotFound
	}
	if err != nil {
		return Plan{}, "", err
	}
	p.Date = database.ISOFromDatetime(rawDate)
	p.CreatedAt = database.ISOFromDatetime(p.CreatedAt)
	p.UpdatedAt = database.ISOFromDatetime(p.UpdatedAt)
	return p, rawDate, nil
}

// findPlan は userID の人の予定を1件読む。無いか他人のものなら ErrNotFound。
func findPlan(ctx context.Context, q database.QueryRower, id int64, userID, suffix string) (Plan, error) {
	p, _, err := scanPlan(q.QueryRowContext(ctx,
		"SELECT"+planColumns+" FROM StudyPlan AS p WHERE p.id = ? AND p.userId = ? LIMIT 1"+suffix, id, userID))
	return p, err
}

// CreatePlans は1つの日付に予定をまとめて作り、作った件数を返す。参考書は他人の ID を混ぜられないよう、
// 自分の分だけを許す。行は1本の INSERT で入れる（Node と同じ）。
func CreatePlans(ctx context.Context, db *sql.DB, userID string, date time.Time, items []PlanItem, now time.Time) (int64, error) {
	if err := checkTextbooksOwned(ctx, db, items, userID); err != nil {
		return 0, err
	}
	args := make([]any, 0, len(items)*10)
	for _, item := range items {
		args = append(args, userID, date, item.Content, item.Subject, item.TextbookID,
			item.RangeStart, item.RangeEnd, item.RangeUnit, now, now)
	}
	// #nosec G202 -- 埋め込むのは行の数ぶん並べた (?, …) だけ。値は args で渡す
	res, err := db.ExecContext(ctx,
		`INSERT INTO StudyPlan
		   (userId, date, content, subject, textbookId,
		    rangeStart, rangeEnd, rangeUnit, createdAt, updatedAt)
		 VALUES `+database.Placeholders(len(items), "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"),
		args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// checkTextbooksOwned は、予定の参考書がすべて userID の人のものかを確かめる（同じ ID は1つに数える）。
func checkTextbooksOwned(ctx context.Context, db *sql.DB, items []PlanItem, userID string) error {
	var args []any
	seen := map[int64]bool{}
	for _, item := range items {
		if id := item.TextbookID; id != nil && !seen[*id] {
			seen[*id] = true
			args = append(args, *id)
		}
	}
	if len(args) == 0 {
		return nil
	}
	var n int
	// #nosec G202 -- 埋め込むのは ID の数ぶん並べた ? だけ。値は args で渡す
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM Textbook WHERE id IN ("+database.Placeholders(len(args), "?")+") AND userId = ?",
		append(args, userID)...,
	).Scan(&n); err != nil {
		return err
	}
	if n != len(args) {
		return ErrTextbooksNotOwned
	}
	return nil
}

// UpdatePlan は userID の人の予定のうち、送られた項目だけを書き換え、書き換えた後の行を返す。
// 参考書は自分のものだけ。実績を記録済みの予定は未完了へ戻せない。
func UpdatePlan(ctx context.Context, db *sql.DB, userID string, id int64, v PlanPatch, now time.Time) (Plan, error) {
	var updated Plan
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := findPlan(ctx, tx, id, userID, " FOR UPDATE"); err != nil {
			return err
		}
		if v.TextbookID.Value != nil {
			if _, err := findTextbookSettings(ctx, tx, *v.TextbookID.Value, userID); err != nil {
				return err
			}
		}
		if v.Done.Present && !*v.Done.Value {
			var n int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM StudyLog WHERE studyPlanId = ?", id).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return ErrPlanHasLog
			}
		}

		var sets []string
		var args []any
		set := func(column string, present bool, value any) {
			if present {
				sets = append(sets, column+" = ?")
				args = append(args, value)
			}
		}
		set("date", v.Date.Present, v.Date.Value)
		set("content", v.Content.Present, v.Content.Value)
		set("subject", v.Subject.Present, v.Subject.Value)
		set("textbookId", v.TextbookID.Present, v.TextbookID.Value)
		set("rangeStart", v.RangeStart.Present, v.RangeStart.Value)
		set("rangeEnd", v.RangeEnd.Present, v.RangeEnd.Value)
		set("rangeUnit", v.RangeUnit.Present, v.RangeUnit.Value)
		set("done", v.Done.Present, v.Done.Value)
		set("updatedAt", true, now)

		// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（set の1つ目）。値は args で ? として渡す
		if _, err := tx.ExecContext(ctx,
			"UPDATE StudyPlan SET "+strings.Join(sets, ", ")+" WHERE id = ? AND userId = ?",
			append(args, id, userID)...); err != nil {
			return err
		}
		var err error
		updated, err = findPlan(ctx, tx, id, userID, "")
		return err
	})
	return updated, err
}

// DeletePlan は userID の人の予定を消す。無いか他人のものなら ErrNotFound。
// ひも付いた実績の studyPlanId は、外部キーの ON DELETE SET NULL で DB が NULL にする。
func DeletePlan(ctx context.Context, db *sql.DB, userID string, id int64) error {
	return deleteOwned(ctx, db, "DELETE FROM StudyPlan WHERE id = ? AND userId = ?", id, userID)
}

// CompletePlan は userID の人の予定を完了にし、同時に実績を1件作る。実績の作成・予定の完了・初回記録の印は
// 必ず一緒に成立させる（片方だけだと「完了なのに実績が無い」などが残る）。Node と同じ1つのトランザクション。
// 実績の日付・科目・参考書は予定のもの。範囲は参考書の逆算設定に合うかを確かめる。
func CompletePlan(ctx context.Context, db *sql.DB, userID string, id int64, in Completion, now time.Time) (Completed, error) {
	var done Completed
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		// 予定の行だけを押さえる（OF p）。実績の表まで押さえると、隣り合う予定を同時に完了したときに、
		// 互いの隙間のロックを待ってデッドロックになり得る。同じ予定を同時に完了したときは、
		// 後の方が studyPlanId の一意制約に当たって ErrAlreadyCompleted になる。
		var logID *int64
		var tb textbookSettings
		var tbID *int64
		plan, rawDate, err := scanPlan(tx.QueryRowContext(ctx,
			"SELECT"+planColumns+`, t.id, t.rangeUnit, t.totalAmount, l.id
			 FROM StudyPlan AS p
			 LEFT JOIN Textbook AS t ON t.id = p.textbookId
			 LEFT JOIN StudyLog AS l ON l.studyPlanId = p.id
			 WHERE p.id = ? AND p.userId = ?
			 LIMIT 1
			 FOR UPDATE OF p`, id, userID),
			&tbID, &tb.rangeUnit, &tb.totalAmount, &logID)
		if err != nil {
			return err
		}
		if logID != nil {
			return ErrAlreadyCompleted
		}
		rangeStart, rangeEnd := in.RangeStart.Or(plan.RangeStart), in.RangeEnd.Or(plan.RangeEnd)
		rangeUnit := in.RangeUnit.Or(plan.RangeUnit)
		if tbID != nil {
			if err := tb.checkRange(rangeEnd, rangeUnit); err != nil {
				return err
			}
		}

		first, err := markFirstStudyLog(ctx, tx, userID, now)
		if err != nil {
			return err
		}
		// 同じ予定の実績が既にあれば、studyPlanId の UNIQUE 制約に当たる。
		inserted, err := tx.ExecContext(ctx,
			`INSERT INTO StudyLog
			   (userId, studyPlanId, date, minutes, subject, textbookId,
			    rangeStart, rangeEnd, rangeUnit, memo, createdAt, updatedAt)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, plan.ID, rawDate, in.Minutes, plan.Subject, plan.TextbookID,
			rangeStart, rangeEnd, rangeUnit, in.Memo, now, now)
		if database.IsMySQLError(err, database.DuplicateEntry) {
			return ErrAlreadyCompleted
		}
		if err != nil {
			return err
		}
		createdID, err := inserted.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE StudyPlan SET done = ?, updatedAt = ? WHERE id = ? AND userId = ?", true, now, plan.ID, userID); err != nil {
			return err
		}

		// INSERT も UPDATE も行を返さないので、応答に使う形を同じトランザクションで読み直す。
		log, _, err := findLog(ctx, tx, createdID, userID, "")
		if err != nil {
			return err
		}
		updated, err := findPlan(ctx, tx, plan.ID, userID, "")
		done = Completed{Log: log, Plan: updated, IsFirstStudyLog: first}
		return err
	})
	return done, err
}
