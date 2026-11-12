package studyrecord

import (
	"context"
	"database/sql"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 学習記録と予定の読み取り（一覧の API・ダッシュボード・予定の完了の応答）。
// 結果の型はモデルが決め、画面の形（apischema）への変換はコントローラー（feature/study）が行う。

// 一覧の件数の上限。期間で絞ったうえでの安全網で、ページングではない。
const (
	MaxLogs  = 1000
	MaxPlans = 1000
)

// Period は「自分のものを、ある期間ぶんだけ」取るときの期間。From・To はどちらも日付（その日の 00:00 UTC）。
// To が nil なら上限なし。
type Period struct {
	From time.Time
	To   *time.Time
}

// where は期間を SQL の条件にする。To の当日ぶんを含めたいので、上限は To の翌日の 00:00 にする
// （date は「その日の 00:00 UTC」で入っている）。
//
// alias は SQL に直接埋める。値ではなく識別子なので ? では渡せない。渡すのは各クエリに書いた
// 固定の別名だけで、利用者の入力は入らない。
func (p Period) where(alias, userID string) (string, []any) {
	cond := alias + ".userId = ? AND " + alias + ".date >= ?"
	args := []any{userID, p.From}
	if p.To != nil {
		cond += " AND " + alias + ".date < ?"
		args = append(args, dates.AddDays(*p.To, 1))
	}
	return cond, args
}

// ReadPeriod はクエリ文字列の from・to（YYYY-MM-DD）を期間にする。省かれた方は defaultFrom・defaultTo
// （defaultTo が nil なら上限なし）。
//
// 形が不正なら *validation.Issue を返す。不正な項目が複数あれば from → to の順に見て、最初の1件を返す。
// 同じキーが2回以上あるときは、文字列ではない（配列）として断る。
//
// 2つ目の戻り値が false のときは「何にも当たらない期間」で、呼ぶ側は空の一覧を返す。
// 形は正しいが暦に無い日付（2026-13-45 など）がそれにあたる。400 にはせず、空の一覧を返す
// （Node の API のときからの決まり。400 に変えるなら画面と契約の確認が要る）。
func ReadPeriod(query map[string][]string, defaultFrom time.Time, defaultTo *time.Time) (Period, bool, error) {
	var raw [2]*string
	for i, key := range []string{"from", "to"} {
		values, ok := query[key]
		switch {
		case !ok:
		case len(values) > 1:
			return Period{}, false, &validation.Issue{Code: "invalid_type", Field: key, Message: "Invalid input: expected string, received array"}
		case !validation.YMDPattern.MatchString(values[0]):
			return Period{}, false, &validation.Issue{Code: "invalid_format", Field: key, Message: validation.YMDMessage}
		default:
			raw[i] = &values[0]
		}
	}
	p := Period{From: defaultFrom, To: defaultTo}
	if raw[0] != nil {
		from, ok := dates.ParseYMD(*raw[0])
		if !ok {
			return Period{}, false, nil
		}
		p.From = from
	}
	if raw[1] != nil {
		to, ok := dates.ParseYMD(*raw[1])
		if !ok {
			return Period{}, false, nil
		}
		p.To = &to
	}
	return p, true, nil
}

// Textbook は一覧に付ける参考書（apischema.Textbook と同じ並びの項目）。
type Textbook struct {
	ID          int64
	MasterID    *int64
	Name        string
	RangeUnit   *string
	Subject     *string
	TargetDate  *string
	TotalAmount *int64
}

// ListedLog は一覧の実績1件。参考書が無ければ Textbook は nil。
type ListedLog struct {
	ID          int64
	Date        string
	Subject     *string
	Minutes     int64
	TextbookID  *int64
	RangeStart  *int64
	RangeEnd    *int64
	RangeUnit   *string
	Memo        *string
	StudyPlanID *int64
	Textbook    *Textbook
}

// ListedPlan は一覧の予定1件。実績が記録済みなら StudyLogID にその ID が入る。
type ListedPlan struct {
	ID         int64
	Date       string
	Content    *string
	Subject    *string
	Done       bool
	TextbookID *int64
	RangeStart *int64
	RangeEnd   *int64
	RangeUnit  *string
	StudyLogID *int64
	Textbook   *Textbook
}

// DailyMinutes は1日ぶんの合計学習時間。
type DailyMinutes struct {
	Date    string
	Minutes int64
}

// 応答に使わない列（userId・createdAt など）も読んでいる。Node の API と速さを比べたときに、
// DB から受け取る量を揃えた名残で、今は減らしてよい。

const listTextbookColumns = `
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

// textbook は参考書を作る。JOIN の相手が居なければ nil。
func (c *textbookCols) textbook() *Textbook {
	if c.id == nil {
		return nil
	}
	tb := &Textbook{
		ID:          *c.id,
		MasterID:    c.masterID,
		Name:        *c.name,
		TotalAmount: c.totalAmount,
		RangeUnit:   c.rangeUnit,
		Subject:     c.subject,
	}
	if c.targetDate != nil {
		iso := database.ISOFromDatetime(*c.targetDate)
		tb.TargetDate = &iso
	}
	return tb
}

// ListLogs は userID の人の実績を、新しい日付から並べる。同じ日付の中は記録した順（id 昇順）。
func ListLogs(ctx context.Context, db *sql.DB, userID string, p Period) ([]ListedLog, error) {
	where, args := p.where("l", userID)
	rows, err := db.QueryContext(ctx,
		"SELECT"+logColumns+","+listTextbookColumns+`
		 FROM StudyLog AS l
		 LEFT JOIN Textbook AS t ON t.id = l.textbookId
		 WHERE `+where+`
		 ORDER BY l.date DESC, l.id ASC
		 LIMIT ?`,
		append(args, MaxLogs)...,
	)
	if err != nil {
		return nil, err
	}
	// rows を閉じないと、接続がプールへ戻らない。
	defer rows.Close()

	// 空でも nil ではなく長さ0にしておく（画面の形にしたとき null ではなく [] になる）。
	logs := make([]ListedLog, 0)
	for rows.Next() {
		var (
			l      ListedLog
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
		l.Date = database.ISOFromDatetime(date)
		l.Textbook = tb.textbook()
		logs = append(logs, l)
	}
	// ループが途中のエラーで止まった場合は、ここで初めて分かる。
	return logs, rows.Err()
}

// ListPlans は userID の人の予定を、古い日付から並べる。同じ日付の中は作った順（id 昇順）。
// 実績は予定1件につき最大1件（StudyLog.studyPlanId が UNIQUE）なので、JOIN しても行は増えない。
func ListPlans(ctx context.Context, db *sql.DB, userID string, p Period) ([]ListedPlan, error) {
	where, args := p.where("p", userID)
	rows, err := db.QueryContext(ctx,
		"SELECT"+planColumns+","+listTextbookColumns+`, l.id AS log_id
		 FROM StudyPlan AS p
		 LEFT JOIN Textbook AS t ON t.id = p.textbookId
		 LEFT JOIN StudyLog AS l ON l.studyPlanId = p.id
		 WHERE `+where+`
		 ORDER BY p.date ASC, p.id ASC
		 LIMIT ?`,
		append(args, MaxPlans)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := make([]ListedPlan, 0)
	for rows.Next() {
		var (
			p      ListedPlan
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
		p.Date = database.ISOFromDatetime(date)
		p.Textbook = tb.textbook()
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

// ListDailyMinutes は userID の人の日ごとの合計学習時間を、新しい日付から返す。
func ListDailyMinutes(ctx context.Context, db *sql.DB, userID string, p Period) ([]DailyMinutes, error) {
	where, args := p.where("l", userID)
	// #nosec G202 -- where は Period.where が固定の列名と ? で作る。値は args で渡す
	rows, err := db.QueryContext(ctx,
		`SELECT l.date, SUM(l.minutes) AS minutes
		 FROM StudyLog AS l
		 WHERE `+where+`
		 GROUP BY l.date
		 ORDER BY l.date DESC
		 LIMIT ?`,
		append(args, MaxLogs)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	daily := make([]DailyMinutes, 0)
	for rows.Next() {
		var (
			d    DailyMinutes
			date string
		)
		// SUM() は DECIMAL で返ってくるが、整数の文字列なので int64 へそのまま入る。
		if err := rows.Scan(&date, &d.Minutes); err != nil {
			return nil, err
		}
		d.Date = database.ISOFromDatetime(date)
		daily = append(daily, d)
	}
	return daily, rows.Err()
}

// ダッシュボードの期間。
const (
	recentDays   = 7   // 実績の明細を使う直近の日数（今日を含む）
	upcomingDays = 7   // 「今週の予定」の幅（今日を含む）
	streakDays   = 365 // 連続記録日数をさかのぼる日数
)

// Overview はダッシュボードの初回表示ぶん。LogPeriod・PlanPeriod は To を必ず持つ。
type Overview struct {
	Month        string // YYYY-MM（日本時間の今月）
	LogPeriod    Period
	Logs         []ListedLog
	PlanPeriod   Period
	Plans        []ListedPlan
	DailyMinutes []DailyMinutes
}

// Dashboard はダッシュボードの初回表示ぶんをまとめて読む。now を引数で受け取るのは、テストで「今日」を固定できるようにするため。
func Dashboard(ctx context.Context, db *sql.DB, userID string, now time.Time) (Overview, error) {
	today := dates.OnTokyo(now)
	start, end := dates.MonthStart(today), dates.MonthEnd(today)

	// 実績は過去だけ。直近7日が月の頭で前月へはみ出すぶんだけ前へ伸ばす。
	logFrom, logTo := dates.Earlier(start, dates.AddDays(today, -(recentDays-1))), end
	// 予定は未来にもある。今週ぶんが月末をまたぐぶんだけ翌月へ伸ばす。
	planFrom, planTo := start, dates.Later(end, dates.AddDays(today, upcomingDays-1))
	dailyFrom := dates.AddDays(today, -(streakDays - 1))

	o := Overview{
		Month:      today.Format("2006-01"),
		LogPeriod:  Period{From: logFrom, To: &logTo},
		PlanPeriod: Period{From: planFrom, To: &planTo},
	}
	// 3本の SQL を同時に投げる。
	// errgroup は、どれか1本が失敗したら ctx を取り消して残りを止め、最初のエラーを返す。
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		o.Logs, err = ListLogs(ctx, db, userID, o.LogPeriod)
		return err
	})
	g.Go(func() (err error) {
		o.Plans, err = ListPlans(ctx, db, userID, o.PlanPeriod)
		return err
	})
	g.Go(func() (err error) {
		o.DailyMinutes, err = ListDailyMinutes(ctx, db, userID, Period{From: dailyFrom})
		return err
	})
	if err := g.Wait(); err != nil {
		return Overview{}, err
	}
	return o, nil
}

// LogWithTextbook は実績と、その参考書の行（無ければ nil）。
type LogWithTextbook struct {
	Log
	Textbook *TextbookRow
}

// TextbookRow は参考書の行（apischema.TextbookRow と同じ並びの項目）。
type TextbookRow struct {
	CreatedAt   string
	ID          int64
	MasterID    *int64
	Name        string
	RangeUnit   *string
	Subject     *string
	TargetDate  *string
	TotalAmount *int64
	UpdatedAt   string
	UserID      string
}

// FindLogWithTextbook は userID の人の実績1件を、参考書の行と一緒に読む。無いか他人のものなら ErrNotFound。
func FindLogWithTextbook(ctx context.Context, db *sql.DB, userID string, id int64) (LogWithTextbook, error) {
	var l LogWithTextbook
	var tbID *int64
	var tb TextbookRow
	var tbUserID, tbName, tbCreated, tbUpdated *string
	err := db.QueryRowContext(ctx,
		"SELECT"+logColumns+`,
		   t.id, t.userId, t.masterId, t.name, t.totalAmount, t.rangeUnit, t.targetDate, t.subject, t.createdAt, t.updatedAt
		 FROM StudyLog AS l
		 LEFT JOIN Textbook AS t ON t.id = l.textbookId
		 WHERE l.id = ? AND l.userId = ?`, id, userID,
	).Scan(&l.ID, &l.UserID, &l.Date, &l.Subject, &l.Minutes, &l.TextbookID,
		&l.RangeStart, &l.RangeEnd, &l.RangeUnit, &l.Memo, &l.StudyPlanID, &l.CreatedAt, &l.UpdatedAt,
		&tbID, &tbUserID, &tb.MasterID, &tbName, &tb.TotalAmount, &tb.RangeUnit, &tb.TargetDate, &tb.Subject, &tbCreated, &tbUpdated)
	if err == sql.ErrNoRows {
		return LogWithTextbook{}, ErrNotFound
	}
	if err != nil {
		return LogWithTextbook{}, err
	}
	l.Date, l.CreatedAt, l.UpdatedAt = database.ISOFromDatetime(l.Date), database.ISOFromDatetime(l.CreatedAt), database.ISOFromDatetime(l.UpdatedAt)
	if tbID != nil {
		tb.ID, tb.UserID, tb.Name = *tbID, *tbUserID, *tbName
		tb.CreatedAt, tb.UpdatedAt = database.ISOFromDatetime(*tbCreated), database.ISOFromDatetime(*tbUpdated)
		if tb.TargetDate != nil {
			*tb.TargetDate = database.ISOFromDatetime(*tb.TargetDate)
		}
		l.Textbook = &tb
	}
	return l, nil
}
