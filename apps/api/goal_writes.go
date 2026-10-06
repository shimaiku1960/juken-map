package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/goal"
)

// 志望校の書き込み（JUK-75）。Node の次の部分にあたる。
//   - routes/goals.ts の POST /api/goals と PUT・PATCH・DELETE /api/goals/:id
//   - services/goal-service.ts の createGoal・updateGoal・applyGoalPatch・deleteGoal・findOwnedGoal
//
// 書き込みは持ち主の internal/write/goal にある（JUK-154）。ここは本文を確かめ、操作を呼び、結果を応答の形にする。
//
// 入力チェックの規則の正は Zod の goalSchema・updateGoalSchema・patchGoalSchema（src/shared/validations/goal.ts）。

// goalStatuses は GOAL_STATUSES（Zod のスキーマに書いた順。z.enum の文言に出る）。
var goalStatuses = []string{"candidate", "decided"}

// facultyIDRule は z.number().int().positive("志望学部を選択してください")。
var facultyIDRule = httpx.NumberRule{Int: true, Positive: true, PositiveMessage: "志望学部を選択してください"}

// goalNoteRule は z.string().max(500, …)（削らない。null は .nullable() で別に受ける）。
var goalNoteRule = httpx.StringRule{Max: 500, MaxMessage: "500文字以内で入力してください"}

// findGoalWithFaculty は登録した志望校を学部・大学つきで読む（画面が学部名・大学名を出すため）。
func (st *goalStore) findGoalWithFaculty(ctx context.Context, id int64) (apischema.FirstChoiceGoal, error) {
	var g apischema.FirstChoiceGoal
	if err := st.db.QueryRowContext(ctx,
		"SELECT"+goalColumns+fromGoalWithFaculty+" WHERE g.id = ?", id,
	).Scan(goalDest(&g)...); err != nil {
		return apischema.FirstChoiceGoal{}, fmt.Errorf("FinalGoal %d が見つかりません: %w", id, err)
	}
	fixGoalDates(&g)
	return g, nil
}

// writeGoalError は持ち主の断る理由を応答にする。他人のものと存在しないものは区別しない。
func writeGoalError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, goal.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "Not found")
	case errors.Is(err, goal.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	default:
		httpx.InternalError(w, r, fmt.Errorf("goals %s: %w", op, err))
	}
}

// create は POST /api/goals。
func (h *goalHandlers) create(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	in := httpx.ReadObject(body.Value())
	facultyID := int64(in.Number("facultyId", facultyIDRule))
	status := in.OptionalEnum("status", goalStatuses, false)
	if in.Reject(w) {
		return
	}
	id, err := goal.Create(r.Context(), h.store.db, s.UserID, facultyID, status.Ptr(), dates.NowMillis())
	if err != nil {
		writeGoalError(w, r, "create", err)
		return
	}
	created, err := h.store.findGoalWithFaculty(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("goals create: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}

// replace は PUT /api/goals/{id}。本文は updateGoalSchema（goalSchema の全項目を任意にしたもの）で、
// status も確かめるが書くのは facultyId だけ（Node と同じ）。入力チェックは自分の志望校かを確かめるより先。
func (h *goalHandlers) replace(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	in := httpx.ReadObject(body.Value())
	facultyID := in.OptionalInt("facultyId", facultyIDRule, false)
	in.OptionalEnum("status", goalStatuses, false)
	if in.Reject(w) {
		return
	}
	updated, err := goal.ReplaceFaculty(r.Context(), h.store.db, s.UserID, id, facultyID.Ptr())
	if err != nil {
		writeGoalError(w, r, "replace", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.GoalFields(updated))
}

// update は PATCH /api/goals/{id}。
func (h *goalHandlers) update(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	in := httpx.ReadObject(body.Value())
	patch := goal.Patch{
		IsFirstChoice: in.OptionalBool("isFirstChoice").Field(),
		Note:          in.OptionalString("note", goalNoteRule, true).Field(),
		Status:        in.OptionalEnum("status", goalStatuses, false).Field(),
	}
	if in.Reject(w) {
		return
	}
	if err := goal.ApplyPatch(r.Context(), h.store.db, s.UserID, id, patch); err != nil {
		writeGoalError(w, r, "patch", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.OkMessage{Message: apischema.OkMessageOK})
}

// delete は DELETE /api/goals/{id}。
func (h *goalHandlers) delete(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	if err := goal.Delete(r.Context(), h.store.db, s.UserID, id); err != nil {
		writeGoalError(w, r, "delete", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.Deleted{Message: apischema.DeletedMessageDeleted})
}
