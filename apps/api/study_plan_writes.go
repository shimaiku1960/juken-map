package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 学習予定の書き込み（JUK-75）。Node の次の部分にあたる。
//   - routes/study-plans.ts の POST・PATCH・DELETE と POST /api/study-plans/:id/complete
//   - services/study-plan-service.ts の createStudyPlans・updateStudyPlan・deleteStudyPlan・completeOwnedStudyPlan
//   - services/textbook-service.ts の countOwnedTextbooks
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

// storedStudyPlan は DB から読んだ予定の行。date は書き戻せるよう DB の文字列のまま持つ。
type storedStudyPlan struct {
	row     StudyPlanRow
	rawDate string
}

const studyPlanRowColumns = `
  p.id, p.userId, p.date, p.content, p.subject, p.done, p.textbookId,
  p.rangeStart, p.rangeEnd, p.rangeUnit, p.createdAt, p.updatedAt`

func (s *storedStudyPlan) dest() []any {
	r := &s.row
	return []any{&r.ID, &r.UserID, &s.rawDate, &r.Content, &r.Subject, &r.Done, &r.TextbookID,
		&r.RangeStart, &r.RangeEnd, &r.RangeUnit, &r.CreatedAt, &r.UpdatedAt}
}

func (s *storedStudyPlan) fixDates() {
	s.row.Date = database.ISOFromDatetime(s.rawDate)
	s.row.CreatedAt = database.ISOFromDatetime(s.row.CreatedAt)
	s.row.UpdatedAt = database.ISOFromDatetime(s.row.UpdatedAt)
}

// findStudyPlan は userID の人の予定を1件読む（Node の findOwnedStudyPlan）。無いか他人のものなら nil。
func findStudyPlan(ctx context.Context, q database.QueryRower, id int64, userID string) (*storedStudyPlan, error) {
	var s storedStudyPlan
	if err := q.QueryRowContext(ctx,
		"SELECT"+studyPlanRowColumns+" FROM StudyPlan AS p WHERE p.id = ? AND p.userId = ? LIMIT 1", id, userID,
	).Scan(s.dest()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.fixDates()
	return &s, nil
}

type studyPlanWriteStore struct {
	db *sql.DB
}

// countOwnedTextbooks は ids のうち自分の参考書の数（Node の countOwnedTextbooks）。
func (st *studyPlanWriteStore) countOwnedTextbooks(ctx context.Context, ids []int64, userID string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	var n int
	err := st.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM Textbook WHERE id IN (?"+strings.Repeat(", ?", len(ids)-1)+") AND userId = ?",
		append(args, userID)...,
	).Scan(&n)
	return n, err
}

// create は1つの日付に予定をまとめて作り、作った件数を返す。行を1本の INSERT で入れる（Node と同じ）。
func (st *studyPlanWriteStore) create(ctx context.Context, userID, date string, items []studyPlanItem) (int64, error) {
	now := nowMillis()
	day := dateFromYMD(date)
	args := make([]any, 0, len(items)*10)
	for _, item := range items {
		args = append(args, userID, day, item.content.ptr(), item.subject.ptr(), item.textbookID.ptr(),
			item.rangeStart.ptr(), item.rangeEnd.ptr(), item.rangeUnit.ptr(), now, now)
	}
	// #nosec G202 -- 埋め込むのは行の数ぶん並べた (?, …) だけ。値は args で渡す
	res, err := st.db.ExecContext(ctx,
		`INSERT INTO StudyPlan
		   (userId, date, content, subject, textbookId,
		    rangeStart, rangeEnd, rangeUnit, createdAt, updatedAt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`+strings.Repeat(", (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", len(items)-1),
		args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// hasLinkedStudyLog は、この予定にひも付いた実績があるか（完了を取り消してよいかの判断）。
func (st *studyPlanWriteStore) hasLinkedStudyLog(ctx context.Context, planID int64) (bool, error) {
	var n int
	err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM StudyLog WHERE studyPlanId = ?", planID).Scan(&n)
	return n > 0, err
}

// update は送られた項目だけを書き換え、書き換えた後の行を返す。userID の人のものだけを変える。
// 列名はこのコードに書いた固定の名前だけで、利用者の入力は値として ? で渡す（Node と同じ）。
func (st *studyPlanWriteStore) update(ctx context.Context, userID string, id int64, v studyPlanUpdate) (StudyPlanRow, error) {
	var sets []string
	var args []any
	set := func(column string, value any) {
		sets = append(sets, column+" = ?")
		args = append(args, value)
	}
	if v.date.present {
		set("date", dateFromYMD(*v.date.value))
	}
	if v.content.present {
		set("content", v.content.ptr())
	}
	if v.subject.present {
		set("subject", v.subject.ptr())
	}
	if v.textbookID.present {
		set("textbookId", v.textbookID.ptr())
	}
	if v.rangeStart.present {
		set("rangeStart", v.rangeStart.ptr())
	}
	if v.rangeEnd.present {
		set("rangeEnd", v.rangeEnd.ptr())
	}
	if v.rangeUnit.present {
		set("rangeUnit", v.rangeUnit.ptr())
	}
	if v.done.present {
		set("done", *v.done.value)
	}
	set("updatedAt", nowMillis())

	// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（set の1つ目）。値は args で ? として渡す
	if _, err := st.db.ExecContext(ctx,
		"UPDATE StudyPlan SET "+strings.Join(sets, ", ")+" WHERE id = ? AND userId = ?",
		append(args, id, userID)...); err != nil {
		return StudyPlanRow{}, err
	}
	updated, err := findStudyPlan(ctx, st.db, id, userID)
	if err != nil {
		return StudyPlanRow{}, err
	}
	if updated == nil {
		return StudyPlanRow{}, fmt.Errorf("StudyPlan %d が見つかりません", id)
	}
	return updated.row, nil
}

// delete は userID の人の予定を消す。ひも付いた実績の studyPlanId は、外部キーの ON DELETE SET NULL で DB が NULL にする。
func (st *studyPlanWriteStore) delete(ctx context.Context, userID string, id int64) error {
	_, err := st.db.ExecContext(ctx, "DELETE FROM StudyPlan WHERE id = ? AND userId = ?", id, userID)
	return err
}

// planForComplete は完了に使う予定。参考書の逆算設定と、実績がもう有るかを一緒に読む。
type planForComplete struct {
	plan     storedStudyPlan
	textbook *ownedTextbook
	hasLog   bool
}

func (st *studyPlanWriteStore) findForComplete(ctx context.Context, id int64, userID string) (*planForComplete, error) {
	var p planForComplete
	var tbID, logID *int64
	var tb ownedTextbook
	err := st.db.QueryRowContext(ctx,
		"SELECT"+studyPlanRowColumns+`, t.id, t.rangeUnit, t.totalAmount, l.id
		 FROM StudyPlan AS p
		 LEFT JOIN Textbook AS t ON t.id = p.textbookId
		 LEFT JOIN StudyLog AS l ON l.studyPlanId = p.id
		 WHERE p.id = ? AND p.userId = ?
		 LIMIT 1`, id, userID,
	).Scan(append(p.plan.dest(), &tbID, &tb.rangeUnit, &tb.totalAmount, &logID)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.plan.fixDates()
	if tbID != nil {
		p.textbook = &tb
	}
	p.hasLog = logID != nil
	return &p, nil
}

// errAlreadyCompleted は、同じ予定の実績がもう有る（同時に完了して一意制約に当たったときも）。
var errAlreadyCompleted = errors.New("この予定の実績はすでに記録されています")

// complete は予定を完了にし、同時に実績を1件作る。実績の作成・予定の完了・初回記録の印は
// 必ず一緒に成立させる（片方だけだと「完了なのに実績が無い」などが残る）。Node と同じ1つのトランザクション。
func (st *studyPlanWriteStore) complete(ctx context.Context, userID string, plan storedStudyPlan, minutes int64,
	rangeStart, rangeEnd *int64, rangeUnit, memo *string,
) (CompletedStudyPlan, error) {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return CompletedStudyPlan{}, err
	}
	defer tx.Rollback()

	now := nowMillis()
	activation, err := tx.ExecContext(ctx,
		"UPDATE `user` SET firstStudyLogAt = ?, updatedAt = ? WHERE id = ? AND firstStudyLogAt IS NULL",
		now, now, userID)
	if err != nil {
		return CompletedStudyPlan{}, err
	}
	activated, err := activation.RowsAffected()
	if err != nil {
		return CompletedStudyPlan{}, err
	}

	// 同じ予定の実績が既にあれば、studyPlanId の UNIQUE 制約に当たる。
	inserted, err := tx.ExecContext(ctx,
		`INSERT INTO StudyLog
		   (userId, studyPlanId, date, minutes, subject, textbookId,
		    rangeStart, rangeEnd, rangeUnit, memo, createdAt, updatedAt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, plan.row.ID, plan.rawDate, minutes, plan.row.Subject, plan.row.TextbookID,
		rangeStart, rangeEnd, rangeUnit, memo, now, now)
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return CompletedStudyPlan{}, errAlreadyCompleted
	}
	if err != nil {
		return CompletedStudyPlan{}, err
	}
	logID, err := inserted.LastInsertId()
	if err != nil {
		return CompletedStudyPlan{}, err
	}
	// 他人の予定なら1行も変わらない。そのときは作った実績ごとロールバックする。
	done, err := tx.ExecContext(ctx,
		"UPDATE StudyPlan SET done = ?, updatedAt = ? WHERE id = ? AND userId = ?", true, now, plan.row.ID, userID)
	if err != nil {
		return CompletedStudyPlan{}, err
	}
	if n, err := done.RowsAffected(); err != nil || n == 0 {
		return CompletedStudyPlan{}, errors.Join(fmt.Errorf("StudyPlan %d が見つかりません", plan.row.ID), err)
	}

	// INSERT も UPDATE も行を返さないので、応答に使う形を同じトランザクションで読み直す。
	log, err := findStudyLogWithTextbook(ctx, tx, logID)
	if err != nil {
		return CompletedStudyPlan{}, err
	}
	updated, err := findStudyPlan(ctx, tx, plan.row.ID, userID)
	if err != nil {
		return CompletedStudyPlan{}, err
	}
	if updated == nil {
		return CompletedStudyPlan{}, fmt.Errorf("StudyPlan %d が見つかりません", plan.row.ID)
	}
	if err := tx.Commit(); err != nil {
		return CompletedStudyPlan{}, err
	}
	return CompletedStudyPlan{Log: log, Plan: updated.row, IsFirstStudyLog: activated == 1}, nil
}

// findStudyLogWithTextbook は実績と、その参考書の行（無ければ null）を読む。
func findStudyLogWithTextbook(ctx context.Context, q database.QueryRower, id int64) (StudyLogWithTextbook, error) {
	var l StudyLogWithTextbook
	var tbID *int64
	var tb TextbookRow
	var tbUserID, tbName, tbCreated, tbUpdated *string
	err := q.QueryRowContext(ctx,
		"SELECT"+studyLogRowColumns+`,
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
	store *studyPlanWriteStore
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
	// 参考書は他人の ID を混ぜられないよう、自分の分だけを許す（同じ ID は1つに数える）。
	var ids []int64
	seen := map[int64]bool{}
	for _, item := range items {
		if id := item.textbookID.ptr(); id != nil && !seen[*id] {
			seen[*id] = true
			ids = append(ids, *id)
		}
	}
	if len(ids) > 0 {
		owned, err := h.store.countOwnedTextbooks(r.Context(), ids, s.UserID)
		if err != nil {
			internalError(w, r, fmt.Errorf("study-plans textbooks: %w", err))
			return
		}
		if owned != len(ids) {
			writeError(w, http.StatusBadRequest, "不正な参考書が含まれています")
			return
		}
	}
	count, err := h.store.create(r.Context(), s.UserID, date, items)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-plans create: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, CreatedCount{Count: count})
}

// ownedPlan は自分の予定を読む。無いか他人のものなら 404 を送って nil を返す。
func (h *studyPlanWriteHandlers) ownedPlan(w http.ResponseWriter, r *http.Request, id int64, s *session) *storedStudyPlan {
	plan, err := findStudyPlan(r.Context(), h.store.db, id, s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-plans find: %w", err))
		return nil
	}
	if plan == nil {
		writeError(w, http.StatusNotFound, "Not found")
	}
	return plan
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
	if h.ownedPlan(w, r, id, s) == nil {
		return
	}
	if textbookID := input.textbookID.ptr(); textbookID != nil {
		owned, err := h.store.countOwnedTextbooks(r.Context(), []int64{*textbookID}, s.UserID)
		if err != nil {
			internalError(w, r, fmt.Errorf("study-plans textbook: %w", err))
			return
		}
		if owned == 0 {
			writeError(w, http.StatusBadRequest, "不正な参考書です")
			return
		}
	}
	// 実績を記録済みの予定は未完了へ戻せない（戻すと実績だけが宙に浮く）。
	if input.done.present && !*input.done.value {
		linked, err := h.store.hasLinkedStudyLog(r.Context(), id)
		if err != nil {
			internalError(w, r, fmt.Errorf("study-plans linked log: %w", err))
			return
		}
		if linked {
			writeError(w, http.StatusConflict, "実績を記録済みの予定は未完了に戻せません")
			return
		}
	}
	updated, err := h.store.update(r.Context(), s.UserID, id, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-plans update: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, updated)
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
	if h.ownedPlan(w, r, id, s) == nil {
		return
	}
	if err := h.store.delete(r.Context(), s.UserID, id); err != nil {
		internalError(w, r, fmt.Errorf("study-plans delete: %w", err))
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
	p, err := h.store.findForComplete(r.Context(), id, s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-plans complete find: %w", err))
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if p.hasLog {
		writeError(w, http.StatusConflict, errAlreadyCompleted.Error())
		return
	}

	// 範囲は送られてきたものを優先し、送られていなければ（キーが無ければ）予定の値を使う。null は「範囲なし」。
	pick := func(sent optional[int64], planned *int64) *int64 {
		if sent.present {
			return sent.ptr()
		}
		return planned
	}
	rangeStart, rangeEnd := pick(input.rangeStart, p.plan.row.RangeStart), pick(input.rangeEnd, p.plan.row.RangeEnd)
	rangeUnit := p.plan.row.RangeUnit
	if input.rangeUnit.present {
		rangeUnit = input.rangeUnit.ptr()
	}
	if p.textbook != nil {
		if message := textbookRangeError(*p.textbook, rangeEnd, rangeUnit); message != "" {
			writeError(w, http.StatusBadRequest, message)
			return
		}
	}

	completed, err := h.store.complete(r.Context(), s.UserID, p.plan, input.minutes, rangeStart, rangeEnd, rangeUnit, input.memo.ptr())
	if errors.Is(err, errAlreadyCompleted) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		internalError(w, r, fmt.Errorf("study-plans complete: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, completed)
}
