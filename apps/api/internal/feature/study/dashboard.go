package study

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/studyrecord"
)

// 応答の形は apischema.Dashboard（src/shared/dto/study.ts の Dashboard）。どの期間を読むかはモデルが決める。

type DashboardHandler struct {
	db *sql.DB
}

// Serve は GET /api/dashboard。
func (h *DashboardHandler) Serve(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	o, err := studyrecord.Dashboard(r.Context(), h.db, s.UserID, time.Now())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("dashboard: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.Dashboard{
		Month:        o.Month,
		LogRange:     apischema.DateRange{From: dates.YMD(o.LogPeriod.From), To: dates.YMD(*o.LogPeriod.To)},
		Logs:         logsView(o.Logs),
		PlanRange:    apischema.DateRange{From: dates.YMD(o.PlanPeriod.From), To: dates.YMD(*o.PlanPeriod.To)},
		Plans:        plansView(o.Plans),
		DailyMinutes: dailyView(o.DailyMinutes),
	})
}
