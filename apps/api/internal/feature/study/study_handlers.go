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

// 学習記録・予定の一覧の API（JUK-73）。
// 書き込みは study_log_writes.go・study_plan_writes.go（JUK-75）。

// 期間を省いて呼ばれたときの既定。画面はどれも明示して呼ぶので、これは古いクライアントや
// 手で叩いたときのためのもの。
const (
	defaultLogDays        = 90  // 実績：今日を含めて遡る日数
	defaultDailyDays      = 365 // 日別の合計：今日を含めて遡る日数
	defaultPlanPastDays   = 90  // 予定：今日から遡る日数
	defaultPlanFutureDays = 90  // 予定：今日から先の日数
)

// Handlers は学習記録・予定・ダッシュボードの入口をまとめたもの。
// ダッシュボードは一覧の API と同じ読み取りを使うので、別の feature に分けずここに置く。
type Handlers struct {
	Reads     *ReadHandlers
	Dashboard *DashboardHandler
	Logs      *LogWriteHandlers
	Plans     *PlanWriteHandlers
}

func New(db *sql.DB) *Handlers {
	return &Handlers{
		Reads:     &ReadHandlers{db: db},
		Dashboard: &DashboardHandler{db: db},
		Logs:      &LogWriteHandlers{db: db},
		Plans:     &PlanWriteHandlers{db: db},
	}
}

type ReadHandlers struct {
	db *sql.DB
}

// readPeriod は ?from=&to= を期間にする。形が不正なら 400 を送って false を返す。
// 暦に無い日付なら空の一覧 empty を 200 で送って false を返す。
func readPeriod[T any](w http.ResponseWriter, r *http.Request, defaultFrom time.Time, defaultTo *time.Time, empty []T) (studyrecord.Period, bool) {
	period, ok, err := studyrecord.ReadPeriod(httpx.ParseQuery(r.URL.RawQuery), defaultFrom, defaultTo)
	if httpx.RejectIssue(w, err) {
		return studyrecord.Period{}, false
	}
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, empty)
		return studyrecord.Period{}, false
	}
	return period, true
}

// ListLogs は GET /api/study-logs。from を省くと直近90日、to を省くと上限なし。
func (h *ReadHandlers) ListLogs(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	today := dates.OnTokyo(time.Now())
	period, ok := readPeriod(w, r, dates.AddDays(today, -(defaultLogDays-1)), nil, []apischema.StudyLog{})
	if !ok {
		return
	}
	logs, err := studyrecord.ListLogs(r.Context(), h.db, s.UserID, period)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-logs: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, logsView(logs))
}

// ListDaily は GET /api/study-logs/daily。日ごとの合計だけを返す軽い方（連続記録日数が使う）。
func (h *ReadHandlers) ListDaily(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	today := dates.OnTokyo(time.Now())
	period, ok := readPeriod(w, r, dates.AddDays(today, -(defaultDailyDays-1)), nil, []apischema.DailyStudyMinutes{})
	if !ok {
		return
	}
	daily, err := studyrecord.ListDailyMinutes(r.Context(), h.db, s.UserID, period)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-logs/daily: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dailyView(daily))
}

// ListPlans は GET /api/study-plans。予定は未来にもあるので、省くと今日の前後90日。
func (h *ReadHandlers) ListPlans(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	today := dates.OnTokyo(time.Now())
	defaultTo := dates.AddDays(today, defaultPlanFutureDays)
	period, ok := readPeriod(w, r, dates.AddDays(today, -defaultPlanPastDays), &defaultTo, []apischema.StudyPlan{})
	if !ok {
		return
	}
	plans, err := studyrecord.ListPlans(r.Context(), h.db, s.UserID, period)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-plans: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, plansView(plans))
}
