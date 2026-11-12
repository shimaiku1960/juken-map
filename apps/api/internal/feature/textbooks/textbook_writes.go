package textbooks

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/textbook"
)

// 参考書の書き込み（JUK-75）。POST /api/textbooks と PATCH /api/textbooks/{id}。
// ここは本文を読んでモデルの操作に渡し、結果か断った理由を HTTP の応答にするだけ。

// writeTextbookError はモデルの断る理由を応答にする。他人のものと存在しないものは区別しない。
func writeTextbookError(w http.ResponseWriter, r *http.Request, op string, err error) {
	if httpx.RejectIssue(w, err) {
		return
	}
	switch {
	case errors.Is(err, textbook.ErrNotFound), errors.Is(err, textbook.ErrMasterNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, textbook.ErrMasterNoMetric):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, textbook.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	default:
		httpx.InternalError(w, r, fmt.Errorf("textbooks %s: %w", op, err))
	}
}

// Create は POST /api/textbooks。
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	created, err := textbook.Create(r.Context(), h.db, s.UserID, body.Value(), dates.NowMillis())
	if err != nil {
		writeTextbookError(w, r, "create", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, apischema.TextbookRow(created))
}

// UpdateProgress は PATCH /api/textbooks/{id}。
func (h *Handlers) UpdateProgress(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	updated, err := textbook.UpdateProgress(r.Context(), h.db, s.UserID, id, body.Value(), dates.NowMillis())
	if err != nil {
		writeTextbookError(w, r, "update", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.TextbookRow(updated))
}
