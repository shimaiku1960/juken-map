package studyrecord

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// LogInput は実績の作成・変更で書く値。Date はその日の 00:00 UTC（JavaScript の new Date("YYYY-MM-DD") と同じ値）。
// 任意の項目は「送られなかった」と null を区別する（変更で「範囲が変わったか」を見るため）。
type LogInput struct {
	Date       time.Time
	Minutes    int64
	Subject    opt.Field[string]
	TextbookID opt.Field[int64]
	RangeStart opt.Field[int64]
	RangeEnd   opt.Field[int64]
	RangeUnit  opt.Field[string]
	Memo       opt.Field[string]
}

// CreatedLog は作った実績と、それが利用者の初めての実績だったか。
type CreatedLog struct {
	Log
	IsFirstStudyLog bool
}

const logColumns = `
  l.id, l.userId, l.date, l.subject, l.minutes, l.textbookId,
  l.rangeStart, l.rangeEnd, l.rangeUnit, l.memo, l.studyPlanId,
  l.createdAt, l.updatedAt`

// findLog は userID の人の実績を1件読む。無いか他人のものなら ErrNotFound。rawDate は書き戻せるよう
// DB の文字列のまま返す。suffix には、変える前に行を押さえる " FOR UPDATE" を渡せる。
func findLog(ctx context.Context, q database.QueryRower, id int64, userID, suffix string) (Log, string, error) {
	var l Log
	var rawDate string
	err := q.QueryRowContext(ctx,
		"SELECT"+logColumns+" FROM StudyLog AS l WHERE l.id = ? AND l.userId = ? LIMIT 1"+suffix, id, userID,
	).Scan(
		&l.ID, &l.UserID, &rawDate, &l.Subject, &l.Minutes, &l.TextbookID,
		&l.RangeStart, &l.RangeEnd, &l.RangeUnit, &l.Memo, &l.StudyPlanID,
		&l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Log{}, "", ErrNotFound
	}
	if err != nil {
		return Log{}, "", err
	}
	l.Date = database.ISOFromDatetime(rawDate)
	l.CreatedAt = database.ISOFromDatetime(l.CreatedAt)
	l.UpdatedAt = database.ISOFromDatetime(l.UpdatedAt)
	return l, rawDate, nil
}

// CreateLog は実績を1件記録する。参考書を指定したら、自分のものか・範囲が逆算設定に合うかを確かめる。
// 初回記録の印付けと同じトランザクションで行う。
func CreateLog(ctx context.Context, db *sql.DB, userID string, in LogInput, now time.Time) (CreatedLog, error) {
	var created CreatedLog
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		if in.TextbookID.Value != nil {
			tb, err := findTextbookSettings(ctx, tx, *in.TextbookID.Value, userID)
			if err != nil {
				return err
			}
			if err := tb.checkRange(in.RangeEnd.Value, in.RangeUnit.Value); err != nil {
				return err
			}
		}
		first, err := markFirstStudyLog(ctx, tx, userID, now)
		if err != nil {
			return err
		}
		inserted, err := tx.ExecContext(ctx,
			`INSERT INTO StudyLog
			   (userId, date, minutes, subject, textbookId,
			    rangeStart, rangeEnd, rangeUnit, memo, createdAt, updatedAt)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, in.Date, in.Minutes, in.Subject.Value, in.TextbookID.Value,
			in.RangeStart.Value, in.RangeEnd.Value, in.RangeUnit.Value, in.Memo.Value, now, now)
		if err != nil {
			return err
		}
		id, err := inserted.LastInsertId()
		if err != nil {
			return err
		}
		// INSERT は行を返さないので、応答に使う形を同じトランザクションで読み直す。
		log, _, err := findLog(ctx, tx, id, userID, "")
		created = CreatedLog{Log: log, IsFirstStudyLog: first}
		return err
	})
	return created, err
}

// UpdateLog は userID の人の実績を書き換え、書き換えた後の行を返す。
//
// 予定から作られた実績は、予定との紐づきを壊す項目（日付・科目・参考書）を今の値のまま書き戻す。
// 参考書の範囲を確かめるのは、範囲か参考書を変えたときだけ。時間・メモだけの修正では、後から変わった
// 参考書の設定を過去の実績へさかのぼって当てはめない。
func UpdateLog(ctx context.Context, db *sql.DB, userID string, id int64, in LogInput, now time.Time) (Log, error) {
	var updated Log
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		current, rawDate, err := findLog(ctx, tx, id, userID, " FOR UPDATE")
		if err != nil {
			return err
		}
		var date any = in.Date
		subject, textbookID := in.Subject.Value, in.TextbookID.Value
		fromPlan := current.StudyPlanID != nil
		if fromPlan {
			date, subject, textbookID = rawDate, current.Subject, current.TextbookID
		}

		rangeChanged := in.RangeStart.Differs(current.RangeStart) ||
			in.RangeEnd.Differs(current.RangeEnd) ||
			in.RangeUnit.Differs(current.RangeUnit)
		textbookChanged := !fromPlan && in.TextbookID.Differs(current.TextbookID)
		if textbookID != nil && (rangeChanged || textbookChanged) {
			tb, err := findTextbookSettings(ctx, tx, *textbookID, userID)
			if err != nil {
				return err
			}
			if err := tb.checkRange(in.RangeEnd.Value, in.RangeUnit.Value); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE StudyLog
			 SET date = ?, minutes = ?, subject = ?, textbookId = ?,
			     rangeStart = ?, rangeEnd = ?, rangeUnit = ?, memo = ?, updatedAt = ?
			 WHERE id = ? AND userId = ?`,
			date, in.Minutes, subject, textbookID,
			in.RangeStart.Value, in.RangeEnd.Value, in.RangeUnit.Value, in.Memo.Value, now,
			id, userID,
		); err != nil {
			return err
		}
		updated, _, err = findLog(ctx, tx, id, userID, "")
		return err
	})
	return updated, err
}

// DeleteLog は userID の人の実績を消す。無いか他人のものなら ErrNotFound。
func DeleteLog(ctx context.Context, db *sql.DB, userID string, id int64) error {
	return deleteOwned(ctx, db, "DELETE FROM StudyLog WHERE id = ? AND userId = ?", id, userID)
}

// deleteOwned は1行を消す DELETE を流し、消えなければ ErrNotFound にする。
func deleteOwned(ctx context.Context, db *sql.DB, query string, id int64, userID string) error {
	res, err := db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
