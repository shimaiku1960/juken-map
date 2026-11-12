package goals

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/goal"
)

// 志望校の書き込み（JUK-75）。POST /api/goals と PUT・PATCH・DELETE /api/goals/{id}。
// ここは本文を読んでモデルの操作に渡し、結果か断った理由を HTTP の応答にするだけ。

// writeGoalError はモデルの断る理由を応答にする。他人のものと存在しないものは区別しない。
func writeGoalError(w http.ResponseWriter, r *http.Request, op string, err error) {
	if httpx.RejectIssue(w, err) {
		return
	}
	switch {
	case errors.Is(err, goal.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "Not found")
	case errors.Is(err, goal.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	default:
		httpx.InternalError(w, r, fmt.Errorf("goals %s: %w", op, err))
	}
}

// Create は POST /api/goals。
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	created, err := goal.Create(r.Context(), h.db, s.UserID, body.Value(), dates.NowMillis())
	if err != nil {
		writeGoalError(w, r, "create", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, firstChoiceView(created))
}

// Replace は PUT /api/goals/{id}。
func (h *Handlers) Replace(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	updated, err := goal.Replace(r.Context(), h.db, s.UserID, id, body.Value())
	if err != nil {
		writeGoalError(w, r, "replace", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.GoalFields(updated))
}

// Update は PATCH /api/goals/{id}。
func (h *Handlers) Update(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	if err := goal.Update(r.Context(), h.db, s.UserID, id, body.Value()); err != nil {
		writeGoalError(w, r, "patch", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.OkMessage{Message: apischema.OkMessageOK})
}

// Delete は DELETE /api/goals/{id}。
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	if err := goal.Delete(r.Context(), h.db, s.UserID, id); err != nil {
		writeGoalError(w, r, "delete", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.Deleted{Message: apischema.DeletedMessageDeleted})
}
