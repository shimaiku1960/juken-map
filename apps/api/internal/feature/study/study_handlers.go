package study

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// 学習記録・予定の一覧の API（JUK-73）。Node の routes/study-logs.ts・study-plans.ts の GET にあたる。
// 書き込みは study_log_writes.go・study_plan_writes.go（JUK-75）。

// 期間を省いて呼ばれたときの既定。画面はどれも明示して呼ぶので、これは古いクライアントや
// 手で叩いたときのためのもの。Node と同じ値。
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
	store := &store{db: db}
	return &Handlers{
		Reads:     &ReadHandlers{store: store},
		Dashboard: &DashboardHandler{store: store},
		Logs:      &LogWriteHandlers{db: db, now: time.Now},
		Plans:     &PlanWriteHandlers{db: db},
	}
}

type ReadHandlers struct {
	store *store
}

// ListLogs は GET /api/study-logs。from を省くと直近90日、to を省くと上限なし。
func (h *ReadHandlers) ListLogs(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	q, ok := readDateRangeQuery(w, r)
	if !ok {
		return
	}
	today := dates.OnTokyo(time.Now())
	rng, ok := q.resolve(dates.AddDays(today, -(defaultLogDays-1)), nil)
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, []apischema.StudyLog{})
		return
	}
	logs, err := h.store.listStudyLogs(r.Context(), s.UserID, rng)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-logs: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, logs)
}

// ListDaily は GET /api/study-logs/daily。日ごとの合計だけを返す軽い方（連続記録日数が使う）。
func (h *ReadHandlers) ListDaily(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	q, ok := readDateRangeQuery(w, r)
	if !ok {
		return
	}
	today := dates.OnTokyo(time.Now())
	rng, ok := q.resolve(dates.AddDays(today, -(defaultDailyDays-1)), nil)
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, []apischema.DailyStudyMinutes{})
		return
	}
	daily, err := h.store.listDailyStudyMinutes(r.Context(), s.UserID, rng)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-logs/daily: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, daily)
}

// ListPlans は GET /api/study-plans。予定は未来にもあるので、省くと今日の前後90日。
func (h *ReadHandlers) ListPlans(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	q, ok := readDateRangeQuery(w, r)
	if !ok {
		return
	}
	today := dates.OnTokyo(time.Now())
	defaultTo := dates.AddDays(today, defaultPlanFutureDays)
	rng, ok := q.resolve(dates.AddDays(today, -defaultPlanPastDays), &defaultTo)
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, []apischema.StudyPlan{})
		return
	}
	plans, err := h.store.listStudyPlans(r.Context(), s.UserID, rng)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-plans: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, plans)
}
