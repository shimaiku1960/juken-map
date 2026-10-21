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
		var err error
		created, err = createLogInTx(ctx, tx, userID, in, now)
		return err
	})
	return created, err
}

// createLogInTx は CreateLog のトランザクションの中身（CreateLogOnce と共有する）。
func createLogInTx(ctx context.Context, tx *sql.Tx, userID string, in LogInput, now time.Time) (CreatedLog, error) {
	if in.TextbookID.Value != nil {
		tb, err := findTextbookSettings(ctx, tx, *in.TextbookID.Value, userID)
		if err != nil {
			return CreatedLog{}, err
		}
		if err := tb.checkRange(in.RangeEnd.Value, in.RangeUnit.Value); err != nil {
			return CreatedLog{}, err
		}
	}
	first, err := markFirstStudyLog(ctx, tx, userID, now)
	if err != nil {
		return CreatedLog{}, err
	}
	inserted, err := tx.ExecContext(ctx,
		`INSERT INTO StudyLog
		   (userId, date, minutes, subject, textbookId,
		    rangeStart, rangeEnd, rangeUnit, memo, createdAt, updatedAt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, in.Date, in.Minutes, in.Subject.Value, in.TextbookID.Value,
		in.RangeStart.Value, in.RangeEnd.Value, in.RangeUnit.Value, in.Memo.Value, now, now)
	if err != nil {
		return CreatedLog{}, err
	}
	id, err := inserted.LastInsertId()
	if err != nil {
		return CreatedLog{}, err
	}
	// INSERT は行を返さないので、応答に使う形を同じトランザクションで読み直す。
	log, _, err := findLog(ctx, tx, id, userID, "")
	return CreatedLog{Log: log, IsFirstStudyLog: first}, err
}

// Idempotency は、作成を1回にするための冪等キー（画面が操作ごとに作る）と、本文の SHA-256（16進数）。
type Idempotency struct {
	Key         string
	RequestHash string
}

// errKeyTaken は、キーの行を入れようとして、同じキーの行がもう有ったこと（CreateLogOnce の中だけで使う）。
var errKeyTaken = errors.New("studyrecord: 冪等キーは使用済み")

// CreateLogOnce は CreateLog と同じく実績を1件記録する。ただし同じ利用者・同じキーの2回目以降は新しく作らず、
// 1回目の結果を返す（replayed が true）。キーが同じで本文が違えば ErrIdempotencyKeyReused。
//
// キーの行は、記録と同じトランザクションの最初に入れる。同じキーの2本目は、この INSERT で1本目のコミットを
// 待ち、コミットされたら重複（1062）になる。1本目が失敗して取り消されたら、2本目がそのまま記録する。
func CreateLogOnce(ctx context.Context, db *sql.DB, userID string, idem Idempotency, in LogInput, now time.Time) (created CreatedLog, replayed bool, err error) {
	err = database.InTx(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO IdempotencyKey (userId, idemKey, requestHash, createdAt) VALUES (?, ?, ?, ?)",
			userID, idem.Key, idem.RequestHash, now)
		if database.IsMySQLError(err, database.DuplicateEntry) {
			return errKeyTaken
		}
		if err != nil {
			return err
		}
		created, err = createLogInTx(ctx, tx, userID, in, now)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"UPDATE IdempotencyKey SET studyLogId = ?, isFirstStudyLog = ? WHERE userId = ? AND idemKey = ?",
			created.ID, created.IsFirstStudyLog, userID, idem.Key)
		return err
	})
	if errors.Is(err, errKeyTaken) {
		// 1本目はコミット済み。このトランザクションは取り消したので、新しい読み取りでコミット後の行が見える。
		created, err = replayLog(ctx, db, userID, idem)
		return created, err == nil, err
	}
	return created, false, err
}

// replayLog は、使用済みのキーで1回目に作った実績を返す。本文が1回目と違えば ErrIdempotencyKeyReused。
// 実績の中身は今の行を読むので、1回目のあとに変更されていれば変更後の値になる。
func replayLog(ctx context.Context, db *sql.DB, userID string, idem Idempotency) (CreatedLog, error) {
	var requestHash string
	var studyLogID int64
	var first bool
	err := db.QueryRowContext(ctx,
		"SELECT requestHash, studyLogId, isFirstStudyLog FROM IdempotencyKey WHERE userId = ? AND idemKey = ?",
		userID, idem.Key,
	).Scan(&requestHash, &studyLogID, &first)
	if errors.Is(err, sql.ErrNoRows) {
		// 重複になってから読むまでの間に、実績が消されてキーの行も消えた（ごくまれ）。
		return CreatedLog{}, ErrNotFound
	}
	if err != nil {
		return CreatedLog{}, err
	}
	if requestHash != idem.RequestHash {
		return CreatedLog{}, ErrIdempotencyKeyReused
	}
	log, _, err := findLog(ctx, db, studyLogID, userID, "")
	return CreatedLog{Log: log, IsFirstStudyLog: first}, err
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
