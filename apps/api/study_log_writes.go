package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/studyrecord"
)

// 学習記録（実績）の書き込み（JUK-75）。Node の次の部分にあたる。
//   - routes/study-logs.ts の POST、routes/study-log-item.ts の PATCH・DELETE
//   - services/study-log-service.ts の createStudyLog・updateStudyLog・deleteStudyLog・findOwnedStudyLog
//   - services/textbook-service.ts の findOwnedTextbook、domain/textbookRange.ts
//
// 書き込みと、その決まり（自分の参考書か・範囲が逆算設定に合うか・初回記録の印）は持ち主の
// internal/write/studyrecord にある（JUK-153）。ここは本文を読んで確かめ、操作を呼び、結果を応答の形にする。
//
// 入力チェックの規則の正は Zod の createStudyLogSchema（src/shared/validations/studyLog.ts）。

// studyLogInput は POST・PATCH の本文を読んだもの。任意の項目は「無い」と null を区別する
// （PATCH の「範囲が変わったか」の判定が、Node の undefined !== null に頼っているため）。
type studyLogInput struct {
	date       string
	minutes    int64
	subject    optional[string]
	textbookID optional[int64]
	rangeStart optional[int64]
	rangeEnd   optional[int64]
	rangeUnit  optional[string]
	memo       optional[string]
}

// positiveIntRule は z.number().int().positive()（文言は Zod の既定）。
var positiveIntRule = numberRule{int: true, positive: true}

// minutesRule は実績の時間（分）。実績の記録と予定の完了で同じ。
var minutesRule = numberRule{
	typeMessage:     "学習時間を入力してください",
	int:             true,
	intMessage:      "整数で入力してください",
	positive:        true,
	positiveMessage: "1分以上を入力してください",
	max:             1440,
	maxMessage:      "24時間（1440分）以内で入力してください",
}

// readStudyLogInput は createStudyLogSchema と同じ順に確かめる。today は日本時間の今日（YYYY-MM-DD）。
func readStudyLogInput(body any, today string) (*objectInput, studyLogInput) {
	in := readObject(body)
	var v studyLogInput
	v.date = in.string("date", ymdDateRule("日付を選択してください",
		// Node と同じく文字列のまま比べる（形は ymdDateRule で確かめてある）。
		stringCheck{ok: func(s string) bool { return s <= today }, code: "future_date", message: "未来日は実績として記録できません"}))
	v.minutes = int64(in.number("minutes", minutesRule))
	v.subject = in.optionalString("subject", stringRule{checks: []stringCheck{subjectCheck}}, true)
	v.textbookID = in.optionalInt("textbookId", positiveIntRule, true)
	v.rangeStart = in.optionalInt("rangeStart", positiveIntRule, true)
	v.rangeEnd = in.optionalInt("rangeEnd", positiveIntRule, true)
	v.rangeUnit = in.optionalString("rangeUnit", stringRule{checks: []stringCheck{rangeUnitCheck}}, true)
	v.memo = in.optionalString("memo", memoRule, false)

	in.rangeRules(v.rangeStart, v.rangeEnd, v.rangeUnit)
	return in, v
}

// record は持ち主に渡す形にする。
func (v studyLogInput) record() studyrecord.LogInput {
	return studyrecord.LogInput{
		Date: dateFromYMD(v.date), Minutes: v.minutes, Subject: v.subject.field(), TextbookID: v.textbookID.field(),
		RangeStart: v.rangeStart.field(), RangeEnd: v.rangeEnd.field(), RangeUnit: v.rangeUnit.field(), Memo: v.memo.field(),
	}
}

// rangeRules は superRefine の範囲の3つの規則（実績・予定・予定の完了で同じ）。Zod と同じ順に足す。
func (in *objectInput) rangeRules(start, end optional[int64], unit optional[string]) {
	hasStart, hasEnd := !start.isNull(), !end.isNull()
	if hasStart != hasEnd {
		field := "rangeStart"
		if hasStart {
			field = "rangeEnd"
		}
		in.addIssue("range_incomplete", field, "範囲は開始と終了の両方を入力してください")
	}
	if hasStart && hasEnd && *start.value > *end.value {
		in.addIssue("range_end_before_start", "rangeEnd", "終了は開始以上にしてください")
	}
	if (hasStart || hasEnd) && unit.isNull() {
		in.addIssue("range_unit_required", "rangeUnit", "単位を選択してください")
	}
}

// dateFromYMD は "YYYY-MM-DD"（暦にある日付だと確かめ済み）を、その日の 00:00 UTC にする。
// Node の new Date("YYYY-MM-DD") と同じ値。
func dateFromYMD(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

// writeStudyRecordError は学習記録の操作が断った理由を応答にする。想定外のエラーは op を付けて 500 にする。
func writeStudyRecordError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var rangeErr *studyrecord.RangeError
	switch {
	case errors.Is(err, studyrecord.ErrNotFound):
		writeError(w, http.StatusNotFound, "Not found")
	case errors.Is(err, studyrecord.ErrTextbookNotOwned), errors.Is(err, studyrecord.ErrTextbooksNotOwned), errors.As(err, &rangeErr):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, studyrecord.ErrAlreadyCompleted), errors.Is(err, studyrecord.ErrPlanHasLog):
		writeError(w, http.StatusConflict, err.Error())
	default:
		internalError(w, r, fmt.Errorf("%s: %w", op, err))
	}
}

type studyLogWriteHandlers struct {
	db  *sql.DB
	now func() time.Time
}

func (h *studyLogWriteHandlers) today() string {
	return ymd(dateOnTokyo(h.now()))
}

// create は POST /api/study-logs。
func (h *studyLogWriteHandlers) create(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	in, input := readStudyLogInput(body.value(), h.today())
	if in.reject(w) {
		return
	}
	created, err := studyrecord.CreateLog(r.Context(), h.db, s.UserID, input.record(), nowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-logs create", err)
		return
	}
	writeJSON(w, http.StatusCreated, apischema.CreatedStudyLog{
		ID: created.ID, UserID: created.UserID, Date: created.Date, Subject: created.Subject, Minutes: created.Minutes,
		TextbookID: created.TextbookID, RangeStart: created.RangeStart, RangeEnd: created.RangeEnd, RangeUnit: created.RangeUnit,
		Memo: created.Memo, StudyPlanID: created.StudyPlanID, CreatedAt: created.CreatedAt, UpdatedAt: created.UpdatedAt,
		IsFirstStudyLog: created.IsFirstStudyLog,
	})
}

// update は PATCH /api/study-logs/{id}。Node と同じく、本文は先に読み（415・413 は ID の確かめより先）、
// 自分の実績かを本文の確かめより先に見る（無ければ本文に関わらず 404）。
func (h *studyLogWriteHandlers) update(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var exists bool
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT EXISTS (SELECT 1 FROM StudyLog WHERE id = ? AND userId = ?)", id, s.UserID).Scan(&exists); err != nil {
		internalError(w, r, fmt.Errorf("study-logs find: %w", err))
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	in, input := readStudyLogInput(body.value(), h.today())
	if in.reject(w) {
		return
	}
	updated, err := studyrecord.UpdateLog(r.Context(), h.db, s.UserID, id, input.record(), nowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-logs update", err)
		return
	}
	writeJSON(w, http.StatusOK, apischema.StudyLogRow(updated))
}

// delete は DELETE /api/study-logs/{id}。
func (h *studyLogWriteHandlers) delete(w http.ResponseWriter, r *http.Request, s *session) {
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := studyrecord.DeleteLog(r.Context(), h.db, s.UserID, id); err != nil {
		writeStudyRecordError(w, r, "study-logs delete", err)
		return
	}
	writeJSON(w, http.StatusOK, apischema.Deleted{Message: apischema.DeletedMessageDeleted})
}
