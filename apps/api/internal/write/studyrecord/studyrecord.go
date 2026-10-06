// Package studyrecord は学習記録（実績 StudyLog・予定 StudyPlan・user.firstStudyLogAt）への書き込みの持ち主。
// 外に出すのは操作で、操作ごとに一緒に確定させることと、書き込みの決まり（自分の参考書か、範囲が参考書の
// 逆算設定に合うか、実績のある予定を未完了に戻さない）をトランザクションの中で済ませる。
// 持ち主の一覧と決まりは docs/architecture.md「バックエンドの構成」（JUK-148・JUK-150）。
//
// 入力の形の確かめ（Zod と同じ 400）は入口が行い、ここには確かめ済みの値が来る。
package studyrecord

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 操作が断る理由。入口はこれを応答に直す（ErrNotFound は 404、ほかは文言をそのまま 400・409 に使う）。
var (
	// ErrNotFound は、相手の実績・予定が無いか、他人のものであること。
	ErrNotFound = errors.New("studyrecord: 見つかりません")
	// ErrTextbookNotOwned は、指定した参考書が無いか、他人のものであること。
	ErrTextbookNotOwned = errors.New("不正な参考書です")
	// ErrTextbooksNotOwned は、予定をまとめて作るときに、他人の参考書が混ざっていること。
	ErrTextbooksNotOwned = errors.New("不正な参考書が含まれています")
	// ErrAlreadyCompleted は、同じ予定の実績がもう有ること（同時に完了して一意制約に当たったときも）。
	ErrAlreadyCompleted = errors.New("この予定の実績はすでに記録されています")
	// ErrPlanHasLog は、実績を記録済みの予定を未完了に戻そうとしたこと（戻すと実績だけが宙に浮く）。
	ErrPlanHasLog = errors.New("実績を記録済みの予定は未完了に戻せません")
)

// RangeError は、範囲が参考書の逆算設定に合わないこと。Error() は利用者に見せる文言。
type RangeError struct{ message string }

func (e *RangeError) Error() string { return e.message }

// Log は実績の行。日時は Date#toISOString と同じ形。
// 項目の名前・型・並びを画面に返す形（apischema の StudyLogRow）とそろえ、入口が型の変換だけで返せるようにしている。
type Log struct {
	CreatedAt   string
	Date        string
	ID          int64
	Memo        *string
	Minutes     int64
	RangeEnd    *int64
	RangeStart  *int64
	RangeUnit   *string
	StudyPlanID *int64
	Subject     *string
	TextbookID  *int64
	UpdatedAt   string
	UserID      string
}

// Plan は予定の行。並びは apischema の StudyPlanRow とそろえている。
type Plan struct {
	Content    *string
	CreatedAt  string
	Date       string
	Done       bool
	ID         int64
	RangeEnd   *int64
	RangeStart *int64
	RangeUnit  *string
	Subject    *string
	TextbookID *int64
	UpdatedAt  string
	UserID     string
}

// markFirstStudyLog は、まだなら利用者に初回記録の日時を付け、付けたら true を返す。実績を作る操作が
// 同じトランザクションの中で呼ぶ。UPDATE の WHERE に firstStudyLogAt IS NULL を入れて DB 側で判定させる。
// 先に読んでから書くと、同時に2件記録したときに両方が「初回」になり得る。
func markFirstStudyLog(ctx context.Context, tx *sql.Tx, userID string, now time.Time) (bool, error) {
	res, err := tx.ExecContext(ctx,
		"UPDATE `user` SET firstStudyLogAt = ?, updatedAt = ? WHERE id = ? AND firstStudyLogAt IS NULL",
		now, now, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// textbookSettings は範囲の確かめに使う、参考書の逆算設定。
type textbookSettings struct {
	rangeUnit   *string
	totalAmount *int64
}

// findTextbookSettings は userID の人の参考書の逆算設定を読む。無いか他人のものなら ErrTextbookNotOwned。
func findTextbookSettings(ctx context.Context, q database.QueryRower, id int64, userID string) (textbookSettings, error) {
	var tb textbookSettings
	err := q.QueryRowContext(ctx,
		"SELECT rangeUnit, totalAmount FROM Textbook WHERE id = ? AND userId = ? LIMIT 1", id, userID,
	).Scan(&tb.rangeUnit, &tb.totalAmount)
	if errors.Is(err, sql.ErrNoRows) {
		return tb, ErrTextbookNotOwned
	}
	return tb, err
}

// checkRange は Node の textbookRangeError（domain/textbookRange.ts）と同じ。問題なければ nil。
// rangeEnd・rangeUnit は書く値（無い・null なら nil）。Node の `data.rangeUnit !== textbook.rangeUnit` は、
// undefined でも null でも「違う」になるので、nil はどちらも同じに扱える。
func (tb textbookSettings) checkRange(rangeEnd *int64, rangeUnit *string) error {
	if rangeEnd == nil {
		return nil
	}
	if tb.rangeUnit != nil && (rangeUnit == nil || *rangeUnit != *tb.rangeUnit) {
		return &RangeError{"範囲の単位を参考書の逆算設定に合わせてください"}
	}
	if tb.totalAmount != nil && *rangeEnd > *tb.totalAmount {
		return &RangeError{fmt.Sprintf("終了位置は参考書の総量（%d）以下にしてください", *tb.totalAmount)}
	}
	return nil
}
