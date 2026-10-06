package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 学習記録と予定の読み取り。ダッシュボード（dashboard.go）と一覧の API（study_handlers.go）が共有する。
// Node 側の study-log-service.ts・study-plan-service.ts の list 系にあたる。

// 応答の件数の上限。期間で絞ったうえでの安全網で、ページングではない（Node と同じ値）。
const (
	maxLogs  = 1000
	maxPlans = 1000
)

// ここから下の型が応答の形（src/shared/dto/study.ts）。
// json タグが JSON のキー名になる。NULL になりうる列はポインタにして、nil が null になる。

// dateRange は「自分のものを、ある期間ぶんだけ」取るときの期間。from・to はどちらも日付（その日の 00:00 UTC）。
// to が nil なら上限なし。Node 側の DateRange（services/date-range.ts）にあたる。
type dateRange struct {
	from time.Time
	to   *time.Time
}

// where は期間を SQL の条件にする。to の当日ぶんを含めたいので、上限は to の翌日の 00:00 にする
// （date は「その日の 00:00 UTC」で入っている）。Node の userDateConditions と同じ形。
//
// alias は SQL に直接埋める。値ではなく識別子なので ? では渡せない。渡すのは各クエリに書いた
// 固定の別名だけで、利用者の入力は入らない。
func (d dateRange) where(alias, userID string) (string, []any) {
	cond := alias + ".userId = ? AND " + alias + ".date >= ?"
	args := []any{userID, d.from}
	if d.to != nil {
		cond += " AND " + alias + ".date < ?"
		args = append(args, addDays(*d.to, 1))
	}
	return cond, args
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
func (c *textbookCols) dto() *Textbook {
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

// studyStore は学習記録と予定の SQL をまとめたもの。
type studyStore struct {
	db *sql.DB
}

// listStudyLogs は新しい日付から並べる。同じ日付の中は記録した順（id 昇順）。
func (st *studyStore) listStudyLogs(ctx context.Context, userID string, r dateRange) ([]StudyLog, error) {
	where, args := r.where("l", userID)
	rows, err := st.db.QueryContext(ctx,
		"SELECT"+logColumns+","+textbookColumns+`
		 FROM StudyLog AS l
		 LEFT JOIN Textbook AS t ON t.id = l.textbookId
		 WHERE `+where+`
		 ORDER BY l.date DESC, l.id ASC
		 LIMIT ?`,
		append(args, maxLogs)...,
	)
	if err != nil {
		return nil, err
	}
	// rows を閉じないと、接続がプールへ戻らない。
	defer rows.Close()

	// nil のままだと JSON で null になる。空でも [] を返すため、長さ0で作っておく。
	logs := make([]StudyLog, 0)
	for rows.Next() {
		var (
			l      StudyLog
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
		l.Textbook = tb.dto()
		logs = append(logs, l)
	}
	// ループが途中のエラーで止まった場合は、ここで初めて分かる。
	return logs, rows.Err()
}

// listStudyPlans は古い日付から並べる。同じ日付の中は作った順（id 昇順）。
// 実績は予定1件につき最大1件（StudyLog.studyPlanId が UNIQUE）なので、JOIN しても行は増えない。
func (st *studyStore) listStudyPlans(ctx context.Context, userID string, r dateRange) ([]StudyPlan, error) {
	where, args := r.where("p", userID)
	rows, err := st.db.QueryContext(ctx,
		"SELECT"+planColumns+","+textbookColumns+`, l.id AS log_id
		 FROM StudyPlan AS p
		 LEFT JOIN Textbook AS t ON t.id = p.textbookId
		 LEFT JOIN StudyLog AS l ON l.studyPlanId = p.id
		 WHERE `+where+`
		 ORDER BY p.date ASC, p.id ASC
		 LIMIT ?`,
		append(args, maxPlans)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := make([]StudyPlan, 0)
	for rows.Next() {
		var (
			p      StudyPlan
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
		p.Textbook = tb.dto()
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

// listDailyStudyMinutes は日ごとの合計学習時間を、新しい日付から返す。
func (st *studyStore) listDailyStudyMinutes(ctx context.Context, userID string, r dateRange) ([]DailyStudyMinutes, error) {
	where, args := r.where("l", userID)
	// #nosec G202 -- where は dateRange.where が固定の列名と ? で作る。値は args で渡す
	rows, err := st.db.QueryContext(ctx,
		`SELECT l.date, SUM(l.minutes) AS minutes
		 FROM StudyLog AS l
		 WHERE `+where+`
		 GROUP BY l.date
		 ORDER BY l.date DESC
		 LIMIT ?`,
		append(args, maxLogs)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	daily := make([]DailyStudyMinutes, 0)
	for rows.Next() {
		var (
			d    DailyStudyMinutes
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
