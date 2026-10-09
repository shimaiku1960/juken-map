package study

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// 期間と上限。
const (
	recentDays   = 7   // 実績の明細を使う直近の日数（今日を含む）
	upcomingDays = 7   // 「今週の予定」の幅（今日を含む）
	streakDays   = 365 // 連続記録日数をさかのぼる日数
)

// 応答の形は apischema.Dashboard（src/shared/dto/study.ts の Dashboard）。

type DashboardHandler struct {
	store *store
}

func (h *DashboardHandler) Serve(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	dashboard, err := h.get(r.Context(), s.UserID, time.Now())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("dashboard: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dashboard)
}

// get はダッシュボードの初回表示ぶんをまとめて返す。
// now を引数で受け取るのは、テストで「今日」を固定できるようにするため。
func (h *DashboardHandler) get(ctx context.Context, userID string, now time.Time) (*apischema.Dashboard, error) {
	today := dates.OnTokyo(now)
	start, end := dates.MonthStart(today), dates.MonthEnd(today)

	// 実績は過去だけ。直近7日が月の頭で前月へはみ出すぶんだけ前へ伸ばす。
	logFrom, logTo := dates.Earlier(start, dates.AddDays(today, -(recentDays-1))), end
	// 予定は未来にもある。今週ぶんが月末をまたぐぶんだけ翌月へ伸ばす。
	planFrom, planTo := start, dates.Later(end, dates.AddDays(today, upcomingDays-1))
	dailyFrom := dates.AddDays(today, -(streakDays - 1))

	// 3本の SQL を同時に投げる。
	// errgroup は、どれか1本が失敗したら ctx を取り消して残りを止め、最初のエラーを返す。
	var (
		logs  []apischema.StudyLog
		plans []apischema.StudyPlan
		daily []apischema.DailyStudyMinutes
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

	return &apischema.Dashboard{
		Month:        today.Format("2006-01"),
		LogRange:     apischema.DateRange{From: dates.YMD(logFrom), To: dates.YMD(logTo)},
		Logs:         logs,
		PlanRange:    apischema.DateRange{From: dates.YMD(planFrom), To: dates.YMD(planTo)},
		Plans:        plans,
		DailyMinutes: daily,
	}, nil
}
