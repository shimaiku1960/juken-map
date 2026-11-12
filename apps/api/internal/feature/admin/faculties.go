package admin

import (
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/university"
)

// 管理者ページのマスター編集のうち、学部とタグ（/api/admin/faculties・/api/admin/tags）。
// 共通の部品と全体の決まりは masters.go。

// ListTags は GET /api/admin/tags。
func (h *MasterHandlers) ListTags(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	tags, err := university.Tags(r.Context(), h.db)
	if writeUniversityError(w, r, facultyMessages, "admin tags", err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tagsView(tags))
}

// CreateFaculty は POST /api/admin/faculties。
func (h *MasterHandlers) CreateFaculty(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	created, err := university.CreateFaculty(r.Context(), h.db, body.Value(), dates.NowMillis())
	if writeUniversityError(w, r, facultyMessages, "admin create faculty", err) {
		return
	}
	h.universitiesChanged()
	view := facultyView(created)
	logMasterChange(r.Context(), s.UserID, "create", "Faculty", view.ID, "after", view)
	httpx.WriteJSON(w, http.StatusCreated, view)
}

// UpdateFaculty は PATCH /api/admin/faculties/{id}。
func (h *MasterHandlers) UpdateFaculty(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	change, err := university.UpdateFaculty(r.Context(), h.db, id, body.Value())
	if writeUniversityError(w, r, facultyMessages, "admin update faculty", err) {
		return
	}
	h.universitiesChanged()
	after := facultyView(change.After)
	logMasterChange(r.Context(), s.UserID, "update", "Faculty", id, "before", facultyView(change.Before), "after", after)
	httpx.WriteJSON(w, http.StatusOK, after)
}

// DeleteFaculty は DELETE /api/admin/faculties/{id}。
func (h *MasterHandlers) DeleteFaculty(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	deleted, err := university.DeleteFaculty(r.Context(), h.db, id)
	if writeUniversityError(w, r, facultyMessages, "admin delete faculty", err) {
		return
	}
	h.universitiesChanged()
	logMasterChange(r.Context(), s.UserID, "delete", "Faculty", id, "before", facultyView(deleted))
	w.WriteHeader(http.StatusNoContent)
}
