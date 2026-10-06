package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/opt"
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
	subjectCheck   = stringCheck{ok: func(s string) bool { return StudyLogInputSubject(s).Valid() }, code: "invalid_subject", message: "科目の値が不正です"}
	rangeUnitCheck = stringCheck{ok: func(s string) bool { return StudyLogInputRangeUnit(s).Valid() }, code: "invalid_range_unit", message: "単位の値が不正です"}
	// memoRule は実績のメモ・予定の内容：.max(500).trim()（null は不可）。
	memoRule = stringRule{max: 500, maxMessage: "500文字以内で入力してください", trim: true}
)

// studyPlanItem は予定1件（studyPlanItemSchema）。
type studyPlanItem struct {
	textbookID, rangeStart, rangeEnd optional[int64]
	rangeUnit, content, subject      optional[string]
}

func readStudyPlanItem(in *objectInput) studyPlanItem {
	var v studyPlanItem
	v.textbookID = in.optionalInt("textbookId", positiveIntRule, true)
	v.rangeStart = in.optionalInt("rangeStart", positiveIntRule, true)
	v.rangeEnd = in.optionalInt("rangeEnd", positiveIntRule, true)
	v.rangeUnit = in.optionalString("rangeUnit", stringRule{checks: []stringCheck{rangeUnitCheck}}, true)
	v.content = in.optionalString("content", memoRule, false)
	v.subject = in.optionalString("subject", stringRule{checks: []stringCheck{subjectCheck}}, true)

	in.rangeRules(v.rangeStart, v.rangeEnd, v.rangeUnit)
	// (d) 中身ゼロ（参考書・範囲・メモが全部空）は不可。メモは trim した後の値で見る（Zod と同じ）。
	if v.textbookID.isNull() && v.rangeStart.isNull() && v.rangeEnd.isNull() && (v.content.isNull() || *v.content.value == "") {
		in.addIssue("plan_content_required", "content", "参考書・範囲・メモのいずれかを入力してください")
	}
	return v
}

// readStudyPlansInput は createStudyPlansSchema。要素は番号の順に読み、最初の issue で止まる
// （Zod も要素ごとに、項目の確かめと superRefine を終えてから次の要素へ進む）。
func readStudyPlansInput(body any) (*objectInput, string, []studyPlanItem) {
	in := readObject(body)
	date := in.string("date", ymdDateRule("日付を選択してください"))
	elements := in.array("items", "内容を1つ以上入力してください")
	var items []studyPlanItem
	for i, element := range elements {
		if in.issue != nil {
			break
		}
		item := readObjectAt(element, in.field("items")+"."+strconv.Itoa(i))
		items = append(items, readStudyPlanItem(item))
		in.take(item)
	}
	return in, date, items
}

// studyPlanUpdate は PATCH の本文（updateStudyPlanSchema）。送られた項目だけを書き換える。
type studyPlanUpdate struct {
	date                             optional[string]
	textbookID, rangeStart, rangeEnd optional[int64]
	rangeUnit, content, subject      optional[string]
	done                             optional[bool]
}

func readStudyPlanUpdate(body any) (*objectInput, studyPlanUpdate) {
	in := readObject(body)
	var v studyPlanUpdate
	v.date = in.optionalString("date", ymdDateRule(""), false)
	v.textbookID = in.optionalInt("textbookId", positiveIntRule, true)
	v.rangeStart = in.optionalInt("rangeStart", positiveIntRule, true)
	v.rangeEnd = in.optionalInt("rangeEnd", positiveIntRule, true)
	v.rangeUnit = in.optionalString("rangeUnit", stringRule{checks: []stringCheck{rangeUnitCheck}}, true)
	v.content = in.optionalString("content", memoRule, false)
	v.subject = in.optionalString("subject", stringRule{checks: []stringCheck{subjectCheck}}, true)
	v.done = in.optionalBool("done")
	in.rangeRules(v.rangeStart, v.rangeEnd, v.rangeUnit)
	return in, v
}

// completeInput は予定の完了の本文（completeStudyPlanSchema）。
type completeInput struct {
	minutes              int64
	rangeStart, rangeEnd optional[int64]
	rangeUnit, memo      optional[string]
}

func readCompleteInput(body any) (*objectInput, completeInput) {
	in := readObject(body)
	var v completeInput
	v.minutes = int64(in.number("minutes", minutesRule))
	v.rangeStart = in.optionalInt("rangeStart", positiveIntRule, true)
	v.rangeEnd = in.optionalInt("rangeEnd", positiveIntRule, true)
	v.rangeUnit = in.optionalString("rangeUnit", stringRule{checks: []stringCheck{rangeUnitCheck}}, true)
	v.memo = in.optionalString("memo", memoRule, false)
	in.rangeRules(v.rangeStart, v.rangeEnd, v.rangeUnit)
	return in, v
}

// findStudyLogWithTextbook は実績と、その参考書の行（無ければ null）を読む。
func findStudyLogWithTextbook(ctx context.Context, q database.QueryRower, id int64) (StudyLogWithTextbook, error) {
	var l StudyLogWithTextbook
	var tbID *int64
	var tb TextbookRow
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
		return StudyLogWithTextbook{}, err
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

type studyPlanWriteHandlers struct {
	db *sql.DB
}

// create は POST /api/study-plans。
func (h *studyPlanWriteHandlers) create(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	in, date, items := readStudyPlansInput(body.value())
	if in.reject(w) {
		return
	}
	records := make([]studyrecord.PlanItem, len(items))
	for i, item := range items {
		records[i] = studyrecord.PlanItem{
			TextbookID: item.textbookID.ptr(), RangeStart: item.rangeStart.ptr(), RangeEnd: item.rangeEnd.ptr(),
			RangeUnit: item.rangeUnit.ptr(), Content: item.content.ptr(), Subject: item.subject.ptr(),
		}
	}
	count, err := studyrecord.CreatePlans(r.Context(), h.db, s.UserID, dateFromYMD(date), records, nowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans create", err)
		return
	}
	writeJSON(w, http.StatusCreated, CreatedCount{Count: count})
}

// update は PATCH /api/study-plans/{id}。Node と同じく、入力チェックは自分の予定かを確かめるより先。
func (h *studyPlanWriteHandlers) update(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in, input := readStudyPlanUpdate(body.value())
	if in.reject(w) {
		return
	}
	patch := studyrecord.PlanPatch{
		Date:       opt.Field[time.Time]{Present: input.date.present},
		TextbookID: input.textbookID.field(), RangeStart: input.rangeStart.field(), RangeEnd: input.rangeEnd.field(),
		RangeUnit: input.rangeUnit.field(), Content: input.content.field(), Subject: input.subject.field(),
		Done: input.done.field(),
	}
	if input.date.present {
		day := dateFromYMD(*input.date.value)
		patch.Date.Value = &day
	}
	updated, err := studyrecord.UpdatePlan(r.Context(), h.db, s.UserID, id, patch, nowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans update", err)
		return
	}
	writeJSON(w, http.StatusOK, StudyPlanRow(updated))
}

// delete は DELETE /api/study-plans/{id}。
func (h *studyPlanWriteHandlers) delete(w http.ResponseWriter, r *http.Request, s *session) {
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := studyrecord.DeletePlan(r.Context(), h.db, s.UserID, id); err != nil {
		writeStudyRecordError(w, r, "study-plans delete", err)
		return
	}
	writeJSON(w, http.StatusOK, Deleted{Message: DeletedMessageDeleted})
}

// complete は POST /api/study-plans/{id}/complete。
func (h *studyPlanWriteHandlers) complete(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in, input := readCompleteInput(body.value())
	if in.reject(w) {
		return
	}
	completed, err := studyrecord.CompletePlan(r.Context(), h.db, s.UserID, id, studyrecord.Completion{
		Minutes: input.minutes, RangeStart: input.rangeStart.field(), RangeEnd: input.rangeEnd.field(),
		RangeUnit: input.rangeUnit.field(), Memo: input.memo.ptr(),
	}, nowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans complete", err)
		return
	}
	// 応答の実績には参考書の行を付ける。参考書は画面の形なので、確定した後に入口で読む。
	log, err := findStudyLogWithTextbook(r.Context(), h.db, completed.Log.ID)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-plans complete read: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, CompletedStudyPlan{
		Log: log, Plan: StudyPlanRow(completed.Plan), IsFirstStudyLog: completed.IsFirstStudyLog,
	})
}
