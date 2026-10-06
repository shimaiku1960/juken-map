package study

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
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
	subject    httpx.Optional[string]
	textbookID httpx.Optional[int64]
	rangeStart httpx.Optional[int64]
	rangeEnd   httpx.Optional[int64]
	rangeUnit  httpx.Optional[string]
	memo       httpx.Optional[string]
}

// minutesRule は実績の時間（分）。実績の記録と予定の完了で同じ。
var minutesRule = httpx.NumberRule{
	TypeMessage:     "学習時間を入力してください",
	Int:             true,
	IntMessage:      "整数で入力してください",
	Positive:        true,
	PositiveMessage: "1分以上を入力してください",
	Max:             1440,
	MaxMessage:      "24時間（1440分）以内で入力してください",
}

// readStudyLogInput は createStudyLogSchema と同じ順に確かめる。today は日本時間の今日（YYYY-MM-DD）。
func readStudyLogInput(body any, today string) (*httpx.ObjectInput, studyLogInput) {
	in := httpx.ReadObject(body)
	var v studyLogInput
	v.date = in.String("date", httpx.YMDDateRule("日付を選択してください",
		// Node と同じく文字列のまま比べる（形は httpx.YMDDateRule で確かめてある）。
		httpx.StringCheck{OK: func(s string) bool { return s <= today }, Code: "future_date", Message: "未来日は実績として記録できません"}))
	v.minutes = int64(in.Number("minutes", minutesRule))
	v.subject = in.OptionalString("subject", httpx.StringRule{Checks: []httpx.StringCheck{subjectCheck}}, true)
	v.textbookID = in.OptionalInt("textbookId", httpx.PositiveIntRule, true)
	v.rangeStart = in.OptionalInt("rangeStart", httpx.PositiveIntRule, true)
	v.rangeEnd = in.OptionalInt("rangeEnd", httpx.PositiveIntRule, true)
	v.rangeUnit = in.OptionalString("rangeUnit", httpx.StringRule{Checks: []httpx.StringCheck{rangeUnitCheck}}, true)
	v.memo = in.OptionalString("memo", memoRule, false)

	rangeRules(in, v.rangeStart, v.rangeEnd, v.rangeUnit)
	return in, v
}

// record は持ち主に渡す形にする。
func (v studyLogInput) record() studyrecord.LogInput {
	return studyrecord.LogInput{
		Date: dates.FromYMD(v.date), Minutes: v.minutes, Subject: v.subject.Field(), TextbookID: v.textbookID.Field(),
		RangeStart: v.rangeStart.Field(), RangeEnd: v.rangeEnd.Field(), RangeUnit: v.rangeUnit.Field(), Memo: v.memo.Field(),
	}
}

// rangeRules は superRefine の範囲の3つの規則（実績・予定・予定の完了で同じ）。Zod と同じ順に足す。
func rangeRules(in *httpx.ObjectInput, start, end httpx.Optional[int64], unit httpx.Optional[string]) {
	hasStart, hasEnd := !start.IsNull(), !end.IsNull()
	if hasStart != hasEnd {
		field := "rangeStart"
		if hasStart {
			field = "rangeEnd"
		}
		in.AddIssue("range_incomplete", field, "範囲は開始と終了の両方を入力してください")
	}
	if hasStart && hasEnd && *start.Value > *end.Value {
		in.AddIssue("range_end_before_start", "rangeEnd", "終了は開始以上にしてください")
	}
	if (hasStart || hasEnd) && unit.IsNull() {
		in.AddIssue("range_unit_required", "rangeUnit", "単位を選択してください")
	}
}

// writeStudyRecordError は学習記録の操作が断った理由を応答にする。想定外のエラーは op を付けて 500 にする。
func writeStudyRecordError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var rangeErr *studyrecord.RangeError
	switch {
	case errors.Is(err, studyrecord.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "Not found")
	case errors.Is(err, studyrecord.ErrTextbookNotOwned), errors.Is(err, studyrecord.ErrTextbooksNotOwned), errors.As(err, &rangeErr):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, studyrecord.ErrAlreadyCompleted), errors.Is(err, studyrecord.ErrPlanHasLog):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	default:
		httpx.InternalError(w, r, fmt.Errorf("%s: %w", op, err))
	}
}

type LogWriteHandlers struct {
	db  *sql.DB
	now func() time.Time
}

func (h *LogWriteHandlers) today() string {
	return dates.YMD(dates.OnTokyo(h.now()))
}

// Create は POST /api/study-logs。
func (h *LogWriteHandlers) Create(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	in, input := readStudyLogInput(body.Value(), h.today())
	if in.Reject(w) {
		return
	}
	created, err := studyrecord.CreateLog(r.Context(), h.db, s.UserID, input.record(), dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-logs create", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, apischema.CreatedStudyLog{
		ID: created.ID, UserID: created.UserID, Date: created.Date, Subject: created.Subject, Minutes: created.Minutes,
		TextbookID: created.TextbookID, RangeStart: created.RangeStart, RangeEnd: created.RangeEnd, RangeUnit: created.RangeUnit,
		Memo: created.Memo, StudyPlanID: created.StudyPlanID, CreatedAt: created.CreatedAt, UpdatedAt: created.UpdatedAt,
		IsFirstStudyLog: created.IsFirstStudyLog,
	})
}

// Update は PATCH /api/study-logs/{id}。Node と同じく、本文は先に読み（415・413 は ID の確かめより先）、
// 自分の実績かを本文の確かめより先に見る（無ければ本文に関わらず 404）。
func (h *LogWriteHandlers) Update(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	var exists bool
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT EXISTS (SELECT 1 FROM StudyLog WHERE id = ? AND userId = ?)", id, s.UserID).Scan(&exists); err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-logs find: %w", err))
		return
	}
	if !exists {
		httpx.WriteError(w, http.StatusNotFound, "Not found")
		return
	}
	in, input := readStudyLogInput(body.Value(), h.today())
	if in.Reject(w) {
		return
	}
	updated, err := studyrecord.UpdateLog(r.Context(), h.db, s.UserID, id, input.record(), dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-logs update", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.StudyLogRow(updated))
}

// Delete は DELETE /api/study-logs/{id}。
func (h *LogWriteHandlers) Delete(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	if err := studyrecord.DeleteLog(r.Context(), h.db, s.UserID, id); err != nil {
		writeStudyRecordError(w, r, "study-logs delete", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.Deleted{Message: apischema.DeletedMessageDeleted})
}
