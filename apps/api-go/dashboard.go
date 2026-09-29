package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

// 期間と上限は Node 側（apps/api/src/services/dashboard-service.ts ほか）と同じ値にする。
const (
	recentDays   = 7   // 実績の明細を使う直近の日数（今日を含む）
	upcomingDays = 7   // 「今週の予定」の幅（今日を含む）
	streakDays   = 365 // 連続記録日数をさかのぼる日数
)

// 応答の形（src/shared/dto/study.ts の Dashboard）。学習記録・予定の型は study.go にある。

type dateRangeDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type dashboardDTO struct {
	Month        string            `json:"month"`
	LogRange     dateRangeDTO      `json:"logRange"`
	Logs         []studyLogDTO     `json:"logs"`
	PlanRange    dateRangeDTO      `json:"planRange"`
	Plans        []studyPlanDTO    `json:"plans"`
	DailyMinutes []dailyMinutesDTO `json:"dailyMinutes"`
}

type dashboardHandler struct {
	store *studyStore
}

func (h *dashboardHandler) serve(w http.ResponseWriter, r *http.Request, s *session) {
	dashboard, err := h.get(r.Context(), s.UserID, time.Now())
	if err != nil {
		internalError(w, r, fmt.Errorf("dashboard: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, dashboard)
}

// get はダッシュボードの初回表示ぶんをまとめて返す。Node 側の getDashboard にあたる。
// now を引数で受け取るのは、テストで「今日」を固定できるようにするため。
func (h *dashboardHandler) get(ctx context.Context, userID string, now time.Time) (*dashboardDTO, error) {
	today := dateOnTokyo(now)
	start, end := monthStart(today), monthEnd(today)

	// 実績は過去だけ。直近7日が月の頭で前月へはみ出すぶんだけ前へ伸ばす。
	logFrom, logTo := earlier(start, addDays(today, -(recentDays-1))), end
	// 予定は未来にもある。今週ぶんが月末をまたぐぶんだけ翌月へ伸ばす。
	planFrom, planTo := start, later(end, addDays(today, upcomingDays-1))
	dailyFrom := addDays(today, -(streakDays - 1))

	// 3本の SQL を同時に投げる（Node 側の Promise.all にあたる）。
	// errgroup は、どれか1本が失敗したら ctx を取り消して残りを止め、最初のエラーを返す。
	var (
		logs  []studyLogDTO
		plans []studyPlanDTO
		daily []dailyMinutesDTO
	)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		logs, err = h.store.listStudyLogs(ctx, userID, dateRange{from: logFrom, to: &logTo})
		return err
	})
	g.Go(func() (err error) {
		plans, err = h.store.listStudyPlans(ctx, userID, dateRange{from: planFrom, to: &planTo})
		return err
	})
	g.Go(func() (err error) {
		daily, err = h.store.listDailyStudyMinutes(ctx, userID, dateRange{from: dailyFrom})
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &dashboardDTO{
		Month:        today.Format("2006-01"),
		LogRange:     dateRangeDTO{From: ymd(logFrom), To: ymd(logTo)},
		Logs:         logs,
		PlanRange:    dateRangeDTO{From: ymd(planFrom), To: ymd(planTo)},
		Plans:        plans,
		DailyMinutes: daily,
	}, nil
}
