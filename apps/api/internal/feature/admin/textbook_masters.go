package admin

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/textbookmaster"
)

// 管理者ページのマスター編集のうち、参考書マスター（/api/admin/textbook-masters）のコントローラー。
// 入力の決まり・読み書きはモデル（internal/model/textbookmaster）、応答の形は view.go。共通の部品と全体の決まりは masters.go。

// ---- 入口 ----

// ListTextbookMasters は GET /api/admin/textbook-masters。
func (h *MasterHandlers) ListTextbookMasters(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	masters, err := textbookmaster.List(r.Context(), h.db, httpx.ParseQuery(r.URL.RawQuery))
	if httpx.RejectIssue(w, err) {
		return
	}
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin textbook masters: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, textbookMastersView(masters))
}

// CreateTextbookMaster は POST /api/admin/textbook-masters。
func (h *MasterHandlers) CreateTextbookMaster(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	created, err := textbookmaster.Create(r.Context(), h.db, body.Value(), dates.NowMillis())
	if writeTextbookMasterError(w, r, "admin create textbook master", err) {
		return
	}
	h.textbookMastersChanged()
	view := textbookMasterView(created)
	logMasterChange(r.Context(), s.UserID, "create", "TextbookMaster", view.ID, "after", view)
	httpx.WriteJSON(w, http.StatusCreated, view)
}

// UpdateTextbookMaster は PATCH /api/admin/textbook-masters/{id}。
func (h *MasterHandlers) UpdateTextbookMaster(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	change, err := textbookmaster.Update(r.Context(), h.db, id, body.Value(), dates.NowMillis())
	if writeTextbookMasterError(w, r, "admin update textbook master", err) {
		return
	}
	h.textbookMastersChanged()
	after := textbookMasterView(change.After)
	logMasterChange(r.Context(), s.UserID, "update", "TextbookMaster", id, "before", textbookMasterView(change.Before), "after", after)
	httpx.WriteJSON(w, http.StatusOK, after)
}

// DeleteTextbookMaster は DELETE /api/admin/textbook-masters/{id}。
func (h *MasterHandlers) DeleteTextbookMaster(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	deleted, err := textbookmaster.Delete(r.Context(), h.db, id)
	if writeTextbookMasterError(w, r, "admin delete textbook master", err) {
		return
	}
	h.textbookMastersChanged()
	logMasterChange(r.Context(), s.UserID, "delete", "TextbookMaster", id, "before", textbookMasterView(deleted))
	w.WriteHeader(http.StatusNoContent)
}

// writeTextbookMasterError は、モデルが断った理由を status と文言にして送り、送ったら true を返す。
// 想定外の失敗は 500。err が nil なら何もしない。
func writeTextbookMasterError(w http.ResponseWriter, r *http.Request, op string, err error) bool {
	var inUse *textbookmaster.InUseError
	switch {
	case err == nil:
		return false
	case httpx.RejectIssue(w, err):
	case errors.Is(err, textbookmaster.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, textbookMasterMessages.notFound)
	case errors.Is(err, textbookmaster.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, textbookMasterMessages.duplicate)
	case errors.As(err, &inUse):
		httpx.WriteError(w, http.StatusConflict, fmt.Sprintf(textbookMasterMessages.inUse, inUse.Count))
	default:
		httpx.InternalError(w, r, fmt.Errorf("%s: %w", op, err))
	}
	return true
}
