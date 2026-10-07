package study

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/studyrecord"
)

// 学習予定の書き込み（JUK-75）。Node の次の部分にあたる。
//   - routes/study-plans.ts の POST・PATCH・DELETE と POST /api/study-plans/:id/complete
//   - services/study-plan-service.ts の createStudyPlans・updateStudyPlan・deleteStudyPlan・completeOwnedStudyPlan
//   - services/textbook-service.ts の countOwnedTextbooks
//
// 書き込みと、その決まり（自分の参考書か・範囲・実績のある予定を未完了に戻さない・完了と実績を一緒に確定）は
// 持ち主の internal/write/studyrecord にある（JUK-153）。ここは本文を確かめ、操作を呼び、結果を応答の形にする。
//
// 入力チェックの規則の正は Zod の createStudyPlansSchema・updateStudyPlanSchema（src/shared/validations/studyPlan.ts）と
// completeStudyPlanSchema（studyLog.ts）。

// 科目と範囲の単位の .refine()。値の一覧は契約（openapi/openapi.yaml）の enum から作った型が持つ。
var (
	subjectCheck   = httpx.StringCheck{OK: func(s string) bool { return apischema.StudyLogInputSubject(s).Valid() }, Code: "invalid_subject", Message: "科目の値が不正です"}
	rangeUnitCheck = httpx.StringCheck{OK: func(s string) bool { return apischema.StudyLogInputRangeUnit(s).Valid() }, Code: "invalid_range_unit", Message: "単位の値が不正です"}
	// memoRule は実績のメモ・予定の内容：.max(500).trim()（null は不可）。
	memoRule = httpx.StringRule{Max: 500, MaxMessage: "500文字以内で入力してください", Trim: true}
)

// studyPlanItem は予定1件（studyPlanItemSchema）。
type studyPlanItem struct {
	textbookID, rangeStart, rangeEnd httpx.Optional[int64]
	rangeUnit, content, subject      httpx.Optional[string]
}

func readStudyPlanItem(in *httpx.ObjectInput) studyPlanItem {
	var v studyPlanItem
	v.textbookID = in.OptionalInt("textbookId", httpx.PositiveIntRule, true)
	v.rangeStart = in.OptionalInt("rangeStart", httpx.PositiveIntRule, true)
	v.rangeEnd = in.OptionalInt("rangeEnd", httpx.PositiveIntRule, true)
	v.rangeUnit = in.OptionalString("rangeUnit", httpx.StringRule{Checks: []httpx.StringCheck{rangeUnitCheck}}, true)
	v.content = in.OptionalString("content", memoRule, false)
	v.subject = in.OptionalString("subject", httpx.StringRule{Checks: []httpx.StringCheck{subjectCheck}}, true)

	rangeRules(in, v.rangeStart, v.rangeEnd, v.rangeUnit)
	// (d) 中身ゼロ（参考書・範囲・メモが全部空）は不可。メモは trim した後の値で見る（Zod と同じ）。
	if v.textbookID.IsNull() && v.rangeStart.IsNull() && v.rangeEnd.IsNull() && (v.content.IsNull() || *v.content.Value == "") {
		in.AddIssue("plan_content_required", "content", "参考書・範囲・メモのいずれかを入力してください")
	}
	return v
}

// readStudyPlansInput は createStudyPlansSchema。要素は番号の順に読み、最初の issue で止まる
// （Zod も要素ごとに、項目の確かめと superRefine を終えてから次の要素へ進む）。
func readStudyPlansInput(body any) (*httpx.ObjectInput, string, []studyPlanItem) {
	in := httpx.ReadObject(body)
	date := in.String("date", httpx.YMDDateRule("日付を選択してください"))
	elements := in.Array("items", "内容を1つ以上入力してください")
	var items []studyPlanItem
	for i, element := range elements {
		if in.Issue != nil {
			break
		}
		item := httpx.ReadObjectAt(element, in.Field("items")+"."+strconv.Itoa(i))
		items = append(items, readStudyPlanItem(item))
		in.Take(item)
	}
	return in, date, items
}

// studyPlanUpdate は PATCH の本文（updateStudyPlanSchema）。送られた項目だけを書き換える。
type studyPlanUpdate struct {
	date                             httpx.Optional[string]
	textbookID, rangeStart, rangeEnd httpx.Optional[int64]
	rangeUnit, content, subject      httpx.Optional[string]
	done                             httpx.Optional[bool]
}

func readStudyPlanUpdate(body any) (*httpx.ObjectInput, studyPlanUpdate) {
	in := httpx.ReadObject(body)
	var v studyPlanUpdate
	v.date = in.OptionalString("date", httpx.YMDDateRule(""), false)
	v.textbookID = in.OptionalInt("textbookId", httpx.PositiveIntRule, true)
	v.rangeStart = in.OptionalInt("rangeStart", httpx.PositiveIntRule, true)
	v.rangeEnd = in.OptionalInt("rangeEnd", httpx.PositiveIntRule, true)
	v.rangeUnit = in.OptionalString("rangeUnit", httpx.StringRule{Checks: []httpx.StringCheck{rangeUnitCheck}}, true)
	v.content = in.OptionalString("content", memoRule, false)
	v.subject = in.OptionalString("subject", httpx.StringRule{Checks: []httpx.StringCheck{subjectCheck}}, true)
	v.done = in.OptionalBool("done")
	rangeRules(in, v.rangeStart, v.rangeEnd, v.rangeUnit)
	return in, v
}

// completeInput は予定の完了の本文（completeStudyPlanSchema）。
type completeInput struct {
	minutes              int64
	rangeStart, rangeEnd httpx.Optional[int64]
	rangeUnit, memo      httpx.Optional[string]
}

func readCompleteInput(body any) (*httpx.ObjectInput, completeInput) {
	in := httpx.ReadObject(body)
	var v completeInput
	v.minutes = int64(in.Number("minutes", minutesRule))
	v.rangeStart = in.OptionalInt("rangeStart", httpx.PositiveIntRule, true)
	v.rangeEnd = in.OptionalInt("rangeEnd", httpx.PositiveIntRule, true)
	v.rangeUnit = in.OptionalString("rangeUnit", httpx.StringRule{Checks: []httpx.StringCheck{rangeUnitCheck}}, true)
	v.memo = in.OptionalString("memo", memoRule, false)
	rangeRules(in, v.rangeStart, v.rangeEnd, v.rangeUnit)
	return in, v
}

// findStudyLogWithTextbook は実績と、その参考書の行（無ければ null）を読む。
func findStudyLogWithTextbook(ctx context.Context, q database.QueryRower, id int64) (apischema.StudyLogWithTextbook, error) {
	var l apischema.StudyLogWithTextbook
	var tbID *int64
	var tb apischema.TextbookRow
	var tbUserID, tbName, tbCreated, tbUpdated *string
	err := q.QueryRowContext(ctx,
		`SELECT
			   l.id, l.userId, l.date, l.subject, l.minutes, l.textbookId,
			   l.rangeStart, l.rangeEnd, l.rangeUnit, l.memo, l.studyPlanId, l.createdAt, l.updatedAt,
		   t.id, t.userId, t.masterId, t.name, t.totalAmount, t.rangeUnit, t.targetDate, t.subject, t.createdAt, t.updatedAt
		 FROM StudyLog AS l
		 LEFT JOIN Textbook AS t ON t.id = l.textbookId
		 WHERE l.id = ?`, id,
	).Scan(&l.ID, &l.UserID, &l.Date, &l.Subject, &l.Minutes, &l.TextbookID,
		&l.RangeStart, &l.RangeEnd, &l.RangeUnit, &l.Memo, &l.StudyPlanID, &l.CreatedAt, &l.UpdatedAt,
		&tbID, &tbUserID, &tb.MasterID, &tbName, &tb.TotalAmount, &tb.RangeUnit, &tb.TargetDate, &tb.Subject, &tbCreated, &tbUpdated)
	if err != nil {
		return apischema.StudyLogWithTextbook{}, err
	}
	l.Date, l.CreatedAt, l.UpdatedAt = database.ISOFromDatetime(l.Date), database.ISOFromDatetime(l.CreatedAt), database.ISOFromDatetime(l.UpdatedAt)
	if tbID != nil {
		tb.ID, tb.UserID, tb.Name = *tbID, *tbUserID, *tbName
		tb.CreatedAt, tb.UpdatedAt = database.ISOFromDatetime(*tbCreated), database.ISOFromDatetime(*tbUpdated)
		if tb.TargetDate != nil {
			*tb.TargetDate = database.ISOFromDatetime(*tb.TargetDate)
		}
		l.Textbook = &tb
	}
	return l, nil
}

type PlanWriteHandlers struct {
	db *sql.DB
}

// Create は POST /api/study-plans。
func (h *PlanWriteHandlers) Create(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	in, date, items := readStudyPlansInput(body.Value())
	if in.Reject(w) {
		return
	}
	records := make([]studyrecord.PlanItem, len(items))
	for i, item := range items {
		records[i] = studyrecord.PlanItem{
			TextbookID: item.textbookID.Ptr(), RangeStart: item.rangeStart.Ptr(), RangeEnd: item.rangeEnd.Ptr(),
			RangeUnit: item.rangeUnit.Ptr(), Content: item.content.Ptr(), Subject: item.subject.Ptr(),
		}
	}
	count, err := studyrecord.CreatePlans(r.Context(), h.db, s.UserID, dates.FromYMD(date), records, dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans create", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, apischema.CreatedCount{Count: count})
}

// Update は PATCH /api/study-plans/{id}。Node と同じく、入力チェックは自分の予定かを確かめるより先。
func (h *PlanWriteHandlers) Update(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	in, input := readStudyPlanUpdate(body.Value())
	if in.Reject(w) {
		return
	}
	patch := studyrecord.PlanPatch{
		Date:       opt.Field[time.Time]{Present: input.date.Present},
		TextbookID: input.textbookID.Field(), RangeStart: input.rangeStart.Field(), RangeEnd: input.rangeEnd.Field(),
		RangeUnit: input.rangeUnit.Field(), Content: input.content.Field(), Subject: input.subject.Field(),
		Done: input.done.Field(),
	}
	if input.date.Present {
		day := dates.FromYMD(*input.date.Value)
		patch.Date.Value = &day
	}
	updated, err := studyrecord.UpdatePlan(r.Context(), h.db, s.UserID, id, patch, dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans update", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.StudyPlanRow(updated))
}

// Delete は DELETE /api/study-plans/{id}。
func (h *PlanWriteHandlers) Delete(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	if err := studyrecord.DeletePlan(r.Context(), h.db, s.UserID, id); err != nil {
		writeStudyRecordError(w, r, "study-plans delete", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.Deleted{Message: apischema.DeletedMessageDeleted})
}

// Complete は POST /api/study-plans/{id}/complete。
func (h *PlanWriteHandlers) Complete(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	in, input := readCompleteInput(body.Value())
	if in.Reject(w) {
		return
	}
	completed, err := studyrecord.CompletePlan(r.Context(), h.db, s.UserID, id, studyrecord.Completion{
		Minutes: input.minutes, RangeStart: input.rangeStart.Field(), RangeEnd: input.rangeEnd.Field(),
		RangeUnit: input.rangeUnit.Field(), Memo: input.memo.Ptr(),
	}, dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans complete", err)
		return
	}
	// 応答の実績には参考書の行を付ける。参考書は画面の形なので、確定した後に入口で読む。
	log, err := findStudyLogWithTextbook(r.Context(), h.db, completed.Log.ID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-plans complete read: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, apischema.CompletedStudyPlan{
		Log: log, Plan: apischema.StudyPlanRow(completed.Plan), IsFirstStudyLog: completed.IsFirstStudyLog,
	})
}
