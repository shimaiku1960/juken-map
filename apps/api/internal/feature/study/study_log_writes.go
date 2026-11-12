package study

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/studyrecord"
)

// 学習記録（実績）の入口（JUK-75）。コントローラーにあたる。
//
// 入力の決まり・書き込みの決まり・SQL はモデルの internal/model/studyrecord にある（JUK-153・JUK-219）。
// ここは本文を読んで操作に渡し、結果か断った理由を HTTP の応答にするだけ。

// writeStudyRecordError は学習記録の操作が断った理由を応答にする。想定外のエラーは op を付けて 500 にする。
func writeStudyRecordError(w http.ResponseWriter, r *http.Request, op string, err error) {
	if httpx.RejectIssue(w, err) {
		return
	}
	var rangeErr *studyrecord.RangeError
	switch {
	case errors.Is(err, studyrecord.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "Not found")
	case errors.Is(err, studyrecord.ErrTextbookNotOwned), errors.Is(err, studyrecord.ErrTextbooksNotOwned), errors.As(err, &rangeErr):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, studyrecord.ErrAlreadyCompleted), errors.Is(err, studyrecord.ErrPlanHasLog):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	default:
		httpx.InternalError(w, r, fmt.Errorf("%s: %w", op, err))
	}
}

type LogWriteHandlers struct {
	db *sql.DB
}

// Create は POST /api/study-logs。
func (h *LogWriteHandlers) Create(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	created, err := studyrecord.CreateLog(r.Context(), h.db, s.UserID, body.Value(), dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-logs create", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, apischema.CreatedStudyLog{
		ID: created.ID, UserID: created.UserID, Date: created.Date, Subject: created.Subject, Minutes: created.Minutes,
		TextbookID: created.TextbookID, RangeStart: created.RangeStart, RangeEnd: created.RangeEnd, RangeUnit: created.RangeUnit,
		Memo: created.Memo, StudyPlanID: created.StudyPlanID, CreatedAt: created.CreatedAt, UpdatedAt: created.UpdatedAt,
		IsFirstStudyLog: created.IsFirstStudyLog,
	})
}

// Update は PATCH /api/study-logs/{id}。本文は先に読む（415・413 は ID の確かめより先）。
// 自分の実績か（404）を本文の確かめ（400）より先に見るのは、モデルの操作が受け持つ。
func (h *LogWriteHandlers) Update(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	updated, err := studyrecord.UpdateLog(r.Context(), h.db, s.UserID, id, body.Value(), dates.NowMillis())
	if err != nil {
		writeStudyRecordError(w, r, "study-logs update", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.StudyLogRow(updated))
}

// Delete は DELETE /api/study-logs/{id}。
func (h *LogWriteHandlers) Delete(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	if err := studyrecord.DeleteLog(r.Context(), h.db, s.UserID, id); err != nil {
		writeStudyRecordError(w, r, "study-logs delete", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.Deleted{Message: apischema.DeletedMessageDeleted})
}
