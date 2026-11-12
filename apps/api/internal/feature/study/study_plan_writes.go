package study

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/studyrecord"
)

// 学習予定の入口（JUK-75）。コントローラーにあたる。
//
// 入力の決まり・書き込みの決まり（自分の参考書か・範囲・実績のある予定を未完了に戻さない・完了と実績を一緒に確定）・
// SQL はモデルの internal/model/studyrecord にある（JUK-153・JUK-219）。ここは本文を操作に渡し、結果を応答の形にする。

type PlanWriteHandlers struct {
	db *sql.DB
}

// Create は POST /api/study-plans。
func (h *PlanWriteHandlers) Create(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	count, err := studyrecord.CreatePlans(r.Context(), h.db, s.UserID, body.Value(), dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans create", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, apischema.CreatedCount{Count: count})
}

// Update は PATCH /api/study-plans/{id}。入力の確かめ（400）は自分の予定か（404）より先（モデルの操作が受け持つ）。
func (h *PlanWriteHandlers) Update(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	updated, err := studyrecord.UpdatePlan(r.Context(), h.db, s.UserID, id, body.Value(), dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans update", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.StudyPlanRow(updated))
}

// Delete は DELETE /api/study-plans/{id}。
func (h *PlanWriteHandlers) Delete(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	if err := studyrecord.DeletePlan(r.Context(), h.db, s.UserID, id); err != nil {
		writeStudyRecordError(w, r, "study-plans delete", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.Deleted{Message: apischema.DeletedMessageDeleted})
}

// Complete は POST /api/study-plans/{id}/complete。
func (h *PlanWriteHandlers) Complete(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	completed, err := studyrecord.CompletePlan(r.Context(), h.db, s.UserID, id, body.Value(), dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-plans complete", err)
		return
	}
	// 応答の実績には参考書の行を付ける。確定した後に読み直す。
	log, err := studyrecord.FindLogWithTextbook(r.Context(), h.db, s.UserID, completed.Log.ID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-plans complete read: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, apischema.CompletedStudyPlan{
		Log: logWithTextbookView(log), Plan: apischema.StudyPlanRow(completed.Plan), IsFirstStudyLog: completed.IsFirstStudyLog,
	})
}
