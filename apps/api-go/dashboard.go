package main

import (
	"context"
	"database/sql"
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
	maxLogs      = 1000
	maxPlans     = 1000
)

// ここから下の型が応答の形（src/shared/dto/study.ts）。
// json タグが JSON のキー名になる。NULL になりうる列はポインタにして、nil が null になる。

type textbookDTO struct {
	ID          int64   `json:"id"`
	MasterID    *int64  `json:"masterId"`
	Name        string  `json:"name"`
	TotalAmount *int64  `json:"totalAmount"`
	RangeUnit   *string `json:"rangeUnit"`
	TargetDate  *string `json:"targetDate"`
	Subject     *string `json:"subject"`
}

type studyLogDTO struct {
	ID          int64        `json:"id"`
	Date        string       `json:"date"`
	Minutes     int64        `json:"minutes"`
	Subject     *string      `json:"subject"`
	TextbookID  *int64       `json:"textbookId"`
	Textbook    *textbookDTO `json:"textbook"`
	RangeStart  *int64       `json:"rangeStart"`
	RangeEnd    *int64       `json:"rangeEnd"`
	RangeUnit   *string      `json:"rangeUnit"`
	Memo        *string      `json:"memo"`
	StudyPlanID *int64       `json:"studyPlanId"`
}

type studyPlanDTO struct {
	ID         int64        `json:"id"`
	Date       string       `json:"date"`
	Content    *string      `json:"content"`
	Subject    *string      `json:"subject"`
	Done       bool         `json:"done"`
	StudyLogID *int64       `json:"studyLogId"`
	TextbookID *int64       `json:"textbookId"`
	Textbook   *textbookDTO `json:"textbook"`
	RangeStart *int64       `json:"rangeStart"`
	RangeEnd   *int64       `json:"rangeEnd"`
	RangeUnit  *string      `json:"rangeUnit"`
}

type dailyMinutesDTO struct {
	Date    string `json:"date"`
	Minutes int64  `json:"minutes"`
}

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
	db *sql.DB
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
		logs, err = h.listStudyLogs(ctx, userID, logFrom, logTo)
		return err
	})
	g.Go(func() (err error) {
		plans, err = h.listStudyPlans(ctx, userID, planFrom, planTo)
		return err
	})
	g.Go(func() (err error) {
		daily, err = h.listDailyStudyMinutes(ctx, userID, dailyFrom)
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

// SQL は Node 側（study-columns.ts・study-log-service.ts・study-plan-service.ts）と
// 同じ列・同じ条件にしている。応答に使わない列（userId・createdAt など）も同じく読む。
// 比べたいのは言語の差なので、DB から受け取る量を揃えるため。

const logColumns = `
  l.id, l.userId, l.date, l.subject, l.minutes, l.textbookId,
  l.rangeStart, l.rangeEnd, l.rangeUnit, l.memo, l.studyPlanId,
  l.createdAt, l.updatedAt`

const planColumns = `
  p.id, p.userId, p.date, p.content, p.subject, p.done, p.textbookId,
  p.rangeStart, p.rangeEnd, p.rangeUnit, p.createdAt, p.updatedAt`

const textbookColumns = `
  t.id AS tb_id, t.userId AS tb_userId, t.masterId AS tb_masterId,
  t.name AS tb_name, t.totalAmount AS tb_totalAmount, t.rangeUnit AS tb_rangeUnit,
  t.targetDate AS tb_targetDate, t.subject AS tb_subject,
  t.createdAt AS tb_createdAt, t.updatedAt AS tb_updatedAt`

// textbookCols は LEFT JOIN した参考書の列の受け皿。相手が居なければ全部 NULL になる。
type textbookCols struct {
	id          *int64
	masterID    *int64
	name        *string
	totalAmount *int64
	rangeUnit   *string
	targetDate  *string
	subject     *string
}

// dest は Scan に渡す受け皿。捨てる列（userId・作成日時）は sql.RawBytes で受け、変換しない。
func (c *textbookCols) dest(ignore *sql.RawBytes) []any {
	return []any{
		&c.id, ignore, &c.masterID, &c.name, &c.totalAmount, &c.rangeUnit,
		&c.targetDate, &c.subject, ignore, ignore,
	}
}

// dto は参考書の DTO を作る。JOIN の相手が居なければ nil（JSON では null）。
func (c *textbookCols) dto() *textbookDTO {
	if c.id == nil {
		return nil
	}
	tb := &textbookDTO{
		ID:          *c.id,
		MasterID:    c.masterID,
		Name:        *c.name,
		TotalAmount: c.totalAmount,
		RangeUnit:   c.rangeUnit,
		Subject:     c.subject,
	}
	if c.targetDate != nil {
		iso := isoFromDatetime(*c.targetDate)
		tb.TargetDate = &iso
	}
	return tb
}

func (h *dashboardHandler) listStudyLogs(ctx context.Context, userID string, from, to time.Time) ([]studyLogDTO, error) {
	rows, err := h.db.QueryContext(ctx,
		"SELECT"+logColumns+","+textbookColumns+`
		 FROM StudyLog AS l
		 LEFT JOIN Textbook AS t ON t.id = l.textbookId
		 WHERE l.userId = ? AND l.date >= ? AND l.date < ?
		 ORDER BY l.date DESC, l.id ASC
		 LIMIT ?`,
		userID, from, addDays(to, 1), maxLogs,
	)
	if err != nil {
		return nil, err
	}
	// rows を閉じないと、接続がプールへ戻らない。
	defer rows.Close()

	// nil のままだと JSON で null になる。空でも [] を返すため、長さ0で作っておく。
	logs := make([]studyLogDTO, 0)
	for rows.Next() {
		var (
			l      studyLogDTO
			date   string
			tb     textbookCols
			ignore sql.RawBytes
		)
		dest := []any{
			&l.ID, &ignore, &date, &l.Subject, &l.Minutes, &l.TextbookID,
			&l.RangeStart, &l.RangeEnd, &l.RangeUnit, &l.Memo, &l.StudyPlanID,
			&ignore, &ignore,
		}
		if err := rows.Scan(append(dest, tb.dest(&ignore)...)...); err != nil {
			return nil, err
		}
		l.Date = isoFromDatetime(date)
		l.Textbook = tb.dto()
		logs = append(logs, l)
	}
	// ループが途中のエラーで止まった場合は、ここで初めて分かる。
	return logs, rows.Err()
}

func (h *dashboardHandler) listStudyPlans(ctx context.Context, userID string, from, to time.Time) ([]studyPlanDTO, error) {
	rows, err := h.db.QueryContext(ctx,
		"SELECT"+planColumns+","+textbookColumns+`, l.id AS log_id
		 FROM StudyPlan AS p
		 LEFT JOIN Textbook AS t ON t.id = p.textbookId
		 LEFT JOIN StudyLog AS l ON l.studyPlanId = p.id
		 WHERE p.userId = ? AND p.date >= ? AND p.date < ?
		 ORDER BY p.date ASC, p.id ASC
		 LIMIT ?`,
		userID, from, addDays(to, 1), maxPlans,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := make([]studyPlanDTO, 0)
	for rows.Next() {
		var (
			p      studyPlanDTO
			date   string
			tb     textbookCols
			ignore sql.RawBytes
		)
		dest := []any{
			&p.ID, &ignore, &date, &p.Content, &p.Subject, &p.Done, &p.TextbookID,
			&p.RangeStart, &p.RangeEnd, &p.RangeUnit, &ignore, &ignore,
		}
		dest = append(dest, tb.dest(&ignore)...)
		if err := rows.Scan(append(dest, &p.StudyLogID)...); err != nil {
			return nil, err
		}
		p.Date = isoFromDatetime(date)
		p.Textbook = tb.dto()
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (h *dashboardHandler) listDailyStudyMinutes(ctx context.Context, userID string, from time.Time) ([]dailyMinutesDTO, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT l.date, SUM(l.minutes) AS minutes
		 FROM StudyLog AS l
		 WHERE l.userId = ? AND l.date >= ?
		 GROUP BY l.date
		 ORDER BY l.date DESC
		 LIMIT ?`,
		userID, from, maxLogs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	daily := make([]dailyMinutesDTO, 0)
	for rows.Next() {
		var (
			d    dailyMinutesDTO
			date string
		)
		// SUM() は DECIMAL で返ってくるが、整数の文字列なので int64 へそのまま入る。
		if err := rows.Scan(&date, &d.Minutes); err != nil {
			return nil, err
		}
		d.Date = isoFromDatetime(date)
		daily = append(daily, d)
	}
	return daily, rows.Err()
}
