package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// 志望校の書き込み（JUK-75）。Node の次の部分にあたる。
//   - routes/goals.ts の POST /api/goals と PUT・PATCH・DELETE /api/goals/:id
//   - services/goal-service.ts の createGoal・updateGoal・applyGoalPatch・deleteGoal・findOwnedGoal
//
// 入力チェックの規則の正は Zod の goalSchema・updateGoalSchema・patchGoalSchema（src/shared/validations/goal.ts）。

// goalStatuses は GOAL_STATUSES（Zod のスキーマに書いた順。z.enum の文言に出る）。
var goalStatuses = []string{"candidate", "decided"}

// facultyIDRule は z.number().int().positive("志望学部を選択してください")。
var facultyIDRule = numberRule{int: true, positive: true, positiveMessage: "志望学部を選択してください"}

// goalNoteRule は z.string().max(500, …)（削らない。null は .nullable() で別に受ける）。
var goalNoteRule = stringRule{max: 500, maxMessage: "500文字以内で入力してください"}

var errDuplicateGoal = errors.New("この学部はすでに登録されています")

// goalPatch は PATCH の本文（patchGoalSchema）。送られた項目だけを書き換える。
type goalPatch struct {
	isFirstChoice optional[bool]
	note          optional[string]
	status        optional[string]
}

const goalFieldColumns = "g.id, g.createdAt, g.userId, g.facultyId, g.isFirstChoice, g.note, g.status"

// findGoalFields は志望校の行だけ（学部は付けない）を読む。userID が空でなければ、その人のものだけ
// （Node の findOwnedGoal）。無ければ nil。
func (st *goalStore) findGoalFields(ctx context.Context, id int64, userID string) (*GoalFields, error) {
	query := "SELECT " + goalFieldColumns + " FROM FinalGoal AS g WHERE g.id = ?"
	args := []any{id}
	if userID != "" {
		query += " AND g.userId = ? LIMIT 1"
		args = append(args, userID)
	}
	var g GoalFields
	err := st.db.QueryRowContext(ctx, query, args...).Scan(
		&g.ID, &g.CreatedAt, &g.UserID, &g.FacultyID, &g.IsFirstChoice, &g.Note, &g.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.CreatedAt = isoFromDatetime(g.CreatedAt)
	return &g, nil
}

// createGoal は志望校を登録し、学部・大学つきで返す（画面が学部名・大学名を出すため）。
// 同じ学部の重複は DB の一意制約（userId, facultyId）が弾くので、それを errDuplicateGoal にする。
// 無い学部は外部キーで弾かれ、Node と同じくそのまま 500 になる。
func (st *goalStore) createGoal(ctx context.Context, userID string, facultyID int64, status optional[string]) (*FirstChoiceGoal, error) {
	s := "decided"
	if v := status.ptr(); v != nil {
		s = *v
	}
	res, err := st.db.ExecContext(ctx,
		"INSERT INTO FinalGoal (userId, facultyId, status, createdAt) VALUES (?, ?, ?, ?)",
		userID, facultyID, s, nowMillis())
	if isMySQLError(err, mysqlDuplicateEntry) {
		return nil, errDuplicateGoal
	}
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	var g FirstChoiceGoal
	if err := st.db.QueryRowContext(ctx,
		"SELECT"+goalColumns+fromGoalWithFaculty+" WHERE g.id = ?", id,
	).Scan(goalDest(&g)...); err != nil {
		return nil, fmt.Errorf("FinalGoal %d が見つかりません: %w", id, err)
	}
	fixGoalDates(&g)
	return &g, nil
}

// replaceFaculty は志望校の学部を差し替える（Node の updateGoal）。facultyId が無ければ何も変えない。
// 同じ学部の志望校が既にあると一意制約、無い学部だと外部キーで弾かれ、Node と同じく 500 になる。
func (st *goalStore) replaceFaculty(ctx context.Context, id int64, facultyID optional[int64]) (*GoalFields, error) {
	if v := facultyID.ptr(); v != nil {
		if _, err := st.db.ExecContext(ctx, "UPDATE FinalGoal SET facultyId = ? WHERE id = ?", *v, id); err != nil {
			return nil, err
		}
	}
	g, err := st.findGoalFields(ctx, id, "")
	if err == nil && g == nil {
		err = fmt.Errorf("FinalGoal %d が見つかりません", id)
	}
	return g, err
}

// applyPatch は第一志望・メモ・ステータスのうち、送られてきたものだけを書き換える（Node の applyGoalPatch）。
//
// 第一志望は1ユーザー1校までなので、付け替えは「全部外す→1件立てる」をひとつのトランザクションで行う。
// 分けて実行すると、途中で失敗したときに第一志望が0校の状態が残る。
func (st *goalStore) applyPatch(ctx context.Context, userID string, id int64, p goalPatch) error {
	// 列名はこのコードに書いた固定の名前だけで、利用者の入力は値として ? で渡す。
	var columns []string
	var args []any
	if p.isFirstChoice.present {
		columns, args = append(columns, "isFirstChoice = ?"), append(args, *p.isFirstChoice.value)
	}
	if p.note.present {
		columns, args = append(columns, "note = ?"), append(args, p.note.ptr())
	}
	if p.status.present {
		columns, args = append(columns, "status = ?"), append(args, *p.status.value)
	}
	if len(columns) == 0 {
		return nil
	}
	// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（columns）。値は args で ? として渡す
	update := "UPDATE FinalGoal SET " + strings.Join(columns, ", ") + " WHERE id = ?"
	args = append(args, id)

	if !p.isFirstChoice.present || !*p.isFirstChoice.value {
		// 1文だけなので、トランザクションで包まなくても途中の状態は残らない。
		_, err := st.db.ExecContext(ctx, update, args...)
		return err
	}
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE FinalGoal SET isFirstChoice = FALSE WHERE userId = ?", userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, update, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (st *goalStore) deleteGoal(ctx context.Context, id int64) error {
	_, err := st.db.ExecContext(ctx, "DELETE FROM FinalGoal WHERE id = ?", id)
	return err
}

// ownedGoal は自分の志望校かを確かめる。無いか他人のものなら 404 を送って false を返す。
// 他人のものと存在しないものは区別しない。
func (h *goalHandlers) ownedGoal(w http.ResponseWriter, r *http.Request, id int64, s *session) bool {
	g, err := h.store.findGoalFields(r.Context(), id, s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("goals find: %w", err))
		return false
	}
	if g == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return false
	}
	return true
}

// create は POST /api/goals。
func (h *goalHandlers) create(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	in := readObject(body.value())
	facultyID := int64(in.number("facultyId", facultyIDRule))
	status := in.optionalEnum("status", goalStatuses, false)
	if in.reject(w) {
		return
	}
	created, err := h.store.createGoal(r.Context(), s.UserID, facultyID, status)
	if errors.Is(err, errDuplicateGoal) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		internalError(w, r, fmt.Errorf("goals create: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// replace は PUT /api/goals/{id}。本文は updateGoalSchema（goalSchema の全項目を任意にしたもの）で、
// status も確かめるが書くのは facultyId だけ（Node と同じ）。入力チェックは自分の志望校かを確かめるより先。
func (h *goalHandlers) replace(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in := readObject(body.value())
	facultyID := in.optionalInt("facultyId", facultyIDRule, false)
	in.optionalEnum("status", goalStatuses, false)
	if in.reject(w) {
		return
	}
	if !h.ownedGoal(w, r, id, s) {
		return
	}
	updated, err := h.store.replaceFaculty(r.Context(), id, facultyID)
	if err != nil {
		internalError(w, r, fmt.Errorf("goals replace: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// update は PATCH /api/goals/{id}。
func (h *goalHandlers) update(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in := readObject(body.value())
	patch := goalPatch{
		isFirstChoice: in.optionalBool("isFirstChoice"),
		note:          in.optionalString("note", goalNoteRule, true),
		status:        in.optionalEnum("status", goalStatuses, false),
	}
	if in.reject(w) {
		return
	}
	if !h.ownedGoal(w, r, id, s) {
		return
	}
	if err := h.store.applyPatch(r.Context(), s.UserID, id, patch); err != nil {
		internalError(w, r, fmt.Errorf("goals patch: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, OkMessage{Message: OkMessageOK})
}

// delete は DELETE /api/goals/{id}。
func (h *goalHandlers) delete(w http.ResponseWriter, r *http.Request, s *session) {
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if !h.ownedGoal(w, r, id, s) {
		return
	}
	if err := h.store.deleteGoal(r.Context(), id); err != nil {
		internalError(w, r, fmt.Errorf("goals delete: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, Deleted{Message: DeletedMessageDeleted})
}
