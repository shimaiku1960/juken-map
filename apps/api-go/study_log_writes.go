package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// 学習記録（実績）の書き込み（JUK-75）。Node の次の部分にあたる。
//   - routes/study-logs.ts の POST、routes/study-log-item.ts の PATCH・DELETE
//   - services/study-log-service.ts の createStudyLog・updateStudyLog・deleteStudyLog・findOwnedStudyLog
//   - services/textbook-service.ts の findOwnedTextbook、domain/textbookRange.ts
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

// positiveInt は z.number().int().positive()（文言は Zod の既定）。
var positiveIntRule = numberRule{int: true, positive: true}

// readStudyLogInput は createStudyLogSchema と同じ順に確かめる。today は日本時間の今日（YYYY-MM-DD）。
func readStudyLogInput(body any, today string) (*objectInput, studyLogInput) {
	in := readObject(body)
	var v studyLogInput
	v.date = in.string("date", stringRule{
		min:        1,
		minMessage: "日付を選択してください",
		checks: []stringCheck{
			{ok: ymdPattern.MatchString, code: "invalid_format", message: ymdMessage},
			{ok: func(s string) bool { return !ymdPattern.MatchString(s) || isCalendarYMD(s) }, code: "invalid_date", message: "存在しない日付です"},
			// Node と同じく文字列のまま比べる（形は上で確かめてある）。
			{ok: func(s string) bool { return s <= today }, code: "future_date", message: "未来日は実績として記録できません"},
		},
	})
	v.minutes = int64(in.number("minutes", numberRule{
		typeMessage:     "学習時間を入力してください",
		int:             true,
		intMessage:      "整数で入力してください",
		positive:        true,
		positiveMessage: "1分以上を入力してください",
		max:             1440,
		maxMessage:      "24時間（1440分）以内で入力してください",
	}))
	v.subject = in.optionalString("subject", stringRule{checks: []stringCheck{
		{ok: func(s string) bool { return StudyLogInputSubject(s).Valid() }, code: "invalid_subject", message: "科目の値が不正です"},
	}}, true)
	v.textbookID = in.optionalInt("textbookId", positiveIntRule, true)
	v.rangeStart = in.optionalInt("rangeStart", positiveIntRule, true)
	v.rangeEnd = in.optionalInt("rangeEnd", positiveIntRule, true)
	v.rangeUnit = in.optionalString("rangeUnit", stringRule{checks: []stringCheck{
		{ok: func(s string) bool { return StudyLogInputRangeUnit(s).Valid() }, code: "invalid_range_unit", message: "単位の値が不正です"},
	}}, true)
	v.memo = in.optionalString("memo", stringRule{max: 500, maxMessage: "500文字以内で入力してください", trim: true}, false)

	// superRefine の3つの規則。Zod と同じ順に足す（最初の1件だけが返る）。
	hasStart, hasEnd := !v.rangeStart.isNull(), !v.rangeEnd.isNull()
	if hasStart != hasEnd {
		field := "rangeStart"
		if hasStart {
			field = "rangeEnd"
		}
		in.addIssue("range_incomplete", field, "範囲は開始と終了の両方を入力してください")
	}
	if hasStart && hasEnd && *v.rangeStart.value > *v.rangeEnd.value {
		in.addIssue("range_end_before_start", "rangeEnd", "終了は開始以上にしてください")
	}
	if (hasStart || hasEnd) && v.rangeUnit.isNull() {
		in.addIssue("range_unit_required", "rangeUnit", "単位を選択してください")
	}
	return in, v
}

// ownedTextbook は範囲の確かめに使う、参考書の逆算設定。
type ownedTextbook struct {
	rangeUnit   *string
	totalAmount *int64
}

// textbookRangeError は Node の textbookRangeError と同じ。問題なければ ""。
func textbookRangeError(tb ownedTextbook, in studyLogInput) string {
	if in.rangeEnd.isNull() {
		return ""
	}
	if tb.rangeUnit != nil && in.rangeUnit.differs(tb.rangeUnit) {
		return "範囲の単位を参考書の逆算設定に合わせてください"
	}
	if tb.totalAmount != nil && *in.rangeEnd.value > *tb.totalAmount {
		return fmt.Sprintf("終了位置は参考書の総量（%d）以下にしてください", *tb.totalAmount)
	}
	return ""
}

// storedStudyLog は DB から読んだ実績の行。date は書き戻せるよう DB の文字列のまま持つ。
type storedStudyLog struct {
	row     StudyLogRow
	rawDate string
}

// queryRower は *sql.DB と *sql.Tx の共通部分（トランザクションの中でも外でも読めるように）。
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const studyLogRowColumns = `
  l.id, l.userId, l.date, l.subject, l.minutes, l.textbookId,
  l.rangeStart, l.rangeEnd, l.rangeUnit, l.memo, l.studyPlanId,
  l.createdAt, l.updatedAt`

type studyLogWriteStore struct {
	db *sql.DB
}

// findStudyLog は実績を1件読む。userID が空でなければ、その人のものだけ（Node の findOwnedStudyLog）。
// 無ければ nil。
func findStudyLog(ctx context.Context, q queryRower, id int64, userID string) (*storedStudyLog, error) {
	query := "SELECT" + studyLogRowColumns + " FROM StudyLog AS l WHERE l.id = ?"
	args := []any{id}
	if userID != "" {
		query += " AND l.userId = ? LIMIT 1"
		args = append(args, userID)
	}
	var s storedStudyLog
	r := &s.row
	err := q.QueryRowContext(ctx, query, args...).Scan(
		&r.ID, &r.UserID, &s.rawDate, &r.Subject, &r.Minutes, &r.TextbookID,
		&r.RangeStart, &r.RangeEnd, &r.RangeUnit, &r.Memo, &r.StudyPlanID,
		&r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Date = isoFromDatetime(s.rawDate)
	r.CreatedAt = isoFromDatetime(r.CreatedAt)
	r.UpdatedAt = isoFromDatetime(r.UpdatedAt)
	return &s, nil
}

// findOwnedTextbook は自分の参考書の逆算設定を読む。無いか他人のものなら nil。
func (st *studyLogWriteStore) findOwnedTextbook(ctx context.Context, id int64, userID string) (*ownedTextbook, error) {
	var tb ownedTextbook
	err := st.db.QueryRowContext(ctx,
		"SELECT rangeUnit, totalAmount FROM Textbook WHERE id = ? AND userId = ? LIMIT 1", id, userID,
	).Scan(&tb.rangeUnit, &tb.totalAmount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &tb, err
}

// create は実績を1件記録する。「初回記録」の印付けと同じトランザクションで行う（Node と同じ）。
// UPDATE の WHERE に firstStudyLogAt IS NULL を入れて DB 側で判定させる。先に読んでから書くと、
// 同時に2件記録したときに両方が「初回」になり得る。
func (st *studyLogWriteStore) create(ctx context.Context, userID string, in studyLogInput) (CreatedStudyLog, error) {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return CreatedStudyLog{}, err
	}
	// Commit の後の Rollback は何もしない。途中で返ったときだけ取り消しになる。
	defer tx.Rollback()

	now := nowMillis()
	activation, err := tx.ExecContext(ctx,
		"UPDATE `user` SET firstStudyLogAt = ?, updatedAt = ? WHERE id = ? AND firstStudyLogAt IS NULL",
		now, now, userID)
	if err != nil {
		return CreatedStudyLog{}, err
	}
	activated, err := activation.RowsAffected()
	if err != nil {
		return CreatedStudyLog{}, err
	}
	inserted, err := tx.ExecContext(ctx,
		`INSERT INTO StudyLog
		   (userId, date, minutes, subject, textbookId,
		    rangeStart, rangeEnd, rangeUnit, memo, createdAt, updatedAt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, dateFromYMD(in.date), in.minutes, in.subject.ptr(), in.textbookID.ptr(),
		in.rangeStart.ptr(), in.rangeEnd.ptr(), in.rangeUnit.ptr(), in.memo.ptr(), now, now)
	if err != nil {
		return CreatedStudyLog{}, err
	}
	id, err := inserted.LastInsertId()
	if err != nil {
		return CreatedStudyLog{}, err
	}
	// INSERT は行を返さないので、応答に使う形を同じトランザクションで読み直す。
	created, err := findStudyLog(ctx, tx, id, "")
	if err != nil {
		return CreatedStudyLog{}, err
	}
	if created == nil {
		return CreatedStudyLog{}, fmt.Errorf("StudyLog %d が見つかりません", id)
	}
	if err := tx.Commit(); err != nil {
		return CreatedStudyLog{}, err
	}
	r := created.row
	return CreatedStudyLog{
		ID: r.ID, UserID: r.UserID, Date: r.Date, Subject: r.Subject, Minutes: r.Minutes,
		TextbookID: r.TextbookID, RangeStart: r.RangeStart, RangeEnd: r.RangeEnd, RangeUnit: r.RangeUnit,
		Memo: r.Memo, StudyPlanID: r.StudyPlanID, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		IsFirstStudyLog: activated == 1,
	}, nil
}

// update は実績を書き換え、書き換えた後の行を返す。予定から作られた実績は、予定との紐づきを壊す項目
// （日付・科目・参考書）を今の値のまま書き戻す（Node と同じ）。
func (st *studyLogWriteStore) update(ctx context.Context, current *storedStudyLog, in studyLogInput) (StudyLogRow, error) {
	var date any = dateFromYMD(in.date)
	subject, textbookID := in.subject.ptr(), in.textbookID.ptr()
	if current.row.StudyPlanID != nil {
		date, subject, textbookID = current.rawDate, current.row.Subject, current.row.TextbookID
	}
	if _, err := st.db.ExecContext(ctx,
		`UPDATE StudyLog
		 SET date = ?, minutes = ?, subject = ?, textbookId = ?,
		     rangeStart = ?, rangeEnd = ?, rangeUnit = ?, memo = ?, updatedAt = ?
		 WHERE id = ?`,
		date, in.minutes, subject, textbookID,
		in.rangeStart.ptr(), in.rangeEnd.ptr(), in.rangeUnit.ptr(), in.memo.ptr(), nowMillis(),
		current.row.ID,
	); err != nil {
		return StudyLogRow{}, err
	}
	updated, err := findStudyLog(ctx, st.db, current.row.ID, "")
	if err != nil {
		return StudyLogRow{}, err
	}
	if updated == nil {
		return StudyLogRow{}, fmt.Errorf("StudyLog %d が見つかりません", current.row.ID)
	}
	return updated.row, nil
}

func (st *studyLogWriteStore) delete(ctx context.Context, id int64) error {
	_, err := st.db.ExecContext(ctx, "DELETE FROM StudyLog WHERE id = ?", id)
	return err
}

// dateFromYMD は "YYYY-MM-DD"（暦にある日付だと確かめ済み）を、その日の 00:00 UTC にする。
// Node の new Date("YYYY-MM-DD") と同じ値。
func dateFromYMD(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

type studyLogWriteHandlers struct {
	store *studyLogWriteStore
	now   func() time.Time
}

func (h *studyLogWriteHandlers) today() string {
	return ymd(dateOnTokyo(h.now()))
}

// checkTextbook は参考書を指定したときの確かめ（自分のものか、範囲が逆算設定に合うか）。
// 合わなければ 400 を送って false を返す。
func (h *studyLogWriteHandlers) checkTextbook(w http.ResponseWriter, r *http.Request, textbookID int64, userID string, in studyLogInput) bool {
	tb, err := h.store.findOwnedTextbook(r.Context(), textbookID, userID)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-logs textbook: %w", err))
		return false
	}
	if tb == nil {
		writeError(w, http.StatusBadRequest, "不正な参考書です")
		return false
	}
	if message := textbookRangeError(*tb, in); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return false
	}
	return true
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
	// 参考書を指定する場合は、所有権と逆算設定との整合性を確かめる。
	if !input.textbookID.isNull() && !h.checkTextbook(w, r, *input.textbookID.value, s.UserID, input) {
		return
	}
	created, err := h.store.create(r.Context(), s.UserID, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-logs create: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// owned は path の ID の実績を読む。形が違えば 400、無いか他人のものなら 404 を送って nil を返す。
// Node と同じく、本文は先に読んでおく（415・413 は ID の確かめより先）。
func (h *studyLogWriteHandlers) owned(w http.ResponseWriter, r *http.Request, s *session) *storedStudyLog {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil
	}
	log, err := findStudyLog(r.Context(), h.store.db, id, s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-logs find: %w", err))
		return nil
	}
	if log == nil {
		writeError(w, http.StatusNotFound, "Not found")
	}
	return log
}

// update は PATCH /api/study-logs/{id}。
func (h *studyLogWriteHandlers) update(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	log := h.owned(w, r, s)
	if log == nil {
		return
	}
	in, input := readStudyLogInput(body.value(), h.today())
	if in.reject(w) {
		return
	}

	current := log.row
	rangeChanged := input.rangeStart.differs(current.RangeStart) ||
		input.rangeEnd.differs(current.RangeEnd) ||
		input.rangeUnit.differs(current.RangeUnit)
	fromPlan := current.StudyPlanID != nil
	textbookChanged := !fromPlan && input.textbookID.differs(current.TextbookID)
	effectiveTextbookID := input.textbookID.ptr()
	if fromPlan {
		effectiveTextbookID = current.TextbookID
	}
	// 時間・メモだけの修正では、後から変わった参考書の設定を過去の実績へさかのぼって当てはめない。
	if effectiveTextbookID != nil && (rangeChanged || textbookChanged) &&
		!h.checkTextbook(w, r, *effectiveTextbookID, s.UserID, input) {
		return
	}

	updated, err := h.store.update(r.Context(), log, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-logs update: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// delete は DELETE /api/study-logs/{id}。
func (h *studyLogWriteHandlers) delete(w http.ResponseWriter, r *http.Request, s *session) {
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	log := h.owned(w, r, s)
	if log == nil {
		return
	}
	if err := h.store.delete(r.Context(), log.row.ID); err != nil {
		internalError(w, r, fmt.Errorf("study-logs delete: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, Deleted{Message: DeletedMessageDeleted})
}
