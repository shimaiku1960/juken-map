package admin

import (
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/university"
)

// 管理者ページのマスター編集のうち、大学（/api/admin/universities）。共通の部品と全体の決まりは masters.go。

// ListUniversities は GET /api/admin/universities。
func (h *MasterHandlers) ListUniversities(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	page, err := university.List(r.Context(), h.db, httpx.ParseQuery(r.URL.RawQuery))
	if writeUniversityError(w, r, universityMessages, "admin universities", err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, universityPageView(page))
}

// UniversityDetail は GET /api/admin/universities/{id}。
func (h *MasterHandlers) UniversityDetail(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	detail, err := university.FindDetail(r.Context(), h.db, id)
	if writeUniversityError(w, r, universityMessages, "admin university detail", err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, universityDetailView(detail))
}

// CreateUniversity は POST /api/admin/universities。
func (h *MasterHandlers) CreateUniversity(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	created, err := university.CreateUniversity(r.Context(), h.db, body.Value(), dates.NowMillis())
	if writeUniversityError(w, r, universityMessages, "admin create university", err) {
		return
	}
	h.universitiesChanged()
	view := universityView(created)
	logMasterChange(r.Context(), s.UserID, "create", "University", view.ID, "after", view)
	httpx.WriteJSON(w, http.StatusCreated, view)
}

// UpdateUniversity は PATCH /api/admin/universities/{id}。
func (h *MasterHandlers) UpdateUniversity(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	// path を先に、本文を後に確かめる。
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	change, err := university.UpdateUniversity(r.Context(), h.db, id, body.Value())
	if writeUniversityError(w, r, universityMessages, "admin update university", err) {
		return
	}
	h.universitiesChanged()
	after := universityView(change.After)
	logMasterChange(r.Context(), s.UserID, "update", "University", id, "before", universityView(change.Before), "after", after)
	httpx.WriteJSON(w, http.StatusOK, after)
}

// DeleteUniversity は DELETE /api/admin/universities/{id}。
func (h *MasterHandlers) DeleteUniversity(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	// 本文は使わないが、ほかの書き込みと同じく、受け付けない形の本文なら 415・413 にする。
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	deleted, err := university.DeleteUniversity(r.Context(), h.db, id)
	if writeUniversityError(w, r, universityMessages, "admin delete university", err) {
		return
	}
	h.universitiesChanged()
	logMasterChange(r.Context(), s.UserID, "delete", "University", id, "before", universityView(deleted))
	w.WriteHeader(http.StatusNoContent)
}
