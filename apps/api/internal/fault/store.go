package fault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// 実験の読み込みと、新しい実験の値の確かめ。書き込みは internal/write/chaos。

// 実験の上限。
const (
	// MaxDuration は1つの実験の長さの上限。終わる時刻を必ず決め、長く続けない。
	MaxDuration = time.Hour
	MinDuration = time.Minute
	// MaxDelayMs は待たせる時間の上限。1リクエストの時間の上限（internal/app/server.go の requestTimeout、10秒）より短くし、
	// 待ったあとに応答を返せるようにする。
	MaxDelayMs = 9000
)

// HTTPErrorStatuses は http_error で返せるステータス。
var HTTPErrorStatuses = []int{500, 502, 503, 504}

// Spec は新しく始める実験の中身。
type Spec struct {
	Kind       Kind
	Route      string
	Rate       float64
	DelayMs    int
	StatusCode int
	Duration   time.Duration
}

// Validate は Spec が起こしてよい実験かを確かめる。routes は障害注入の対象にしてよいルート（httpx.Router.FaultRoutes）。
// 誤りの文は入口がそのまま 400 で返す。
func (s Spec) Validate(routes []string) error {
	if !slices.Contains(Kinds, s.Kind) {
		return errors.New("kind が正しくありません")
	}
	if s.Route != AllRoutes && !slices.Contains(routes, s.Route) {
		return errors.New("route は * か、障害を起こしてよいルート（「GET /api/study-logs/:id」の形）にしてください")
	}
	if !(s.Rate > 0 && s.Rate <= 1) {
		return errors.New("rate は 0 より大きく 1 以下にしてください")
	}
	if s.Duration < MinDuration || s.Duration > MaxDuration {
		return fmt.Errorf("durationSeconds は %d〜%d にしてください", int(MinDuration.Seconds()), int(MaxDuration.Seconds()))
	}
	switch s.Kind {
	case KindLatency, KindOutboundTimeout:
		if s.DelayMs < 1 || s.DelayMs > MaxDelayMs {
			return fmt.Errorf("delayMs は 1〜%d にしてください", MaxDelayMs)
		}
		if s.StatusCode != 0 {
			return errors.New("statusCode は http_error のときだけ指定できます")
		}
	case KindHTTPError:
		if !slices.Contains(HTTPErrorStatuses, s.StatusCode) {
			return errors.New("statusCode は 500・502・503・504 のどれかにしてください")
		}
		if s.DelayMs != 0 {
			return errors.New("delayMs は latency・outbound_timeout のときだけ指定できます")
		}
	case KindDBError:
		if s.DelayMs != 0 || s.StatusCode != 0 {
			return errors.New("db_error には delayMs・statusCode を指定できません")
		}
	}
	return nil
}

// Record は記録に残った実験1つ。
type Record struct {
	Experiment
	StoppedAt *time.Time
	StoppedBy string
	StartedBy string // 始めた入口（job・schedule）
}

// Running は now の時点で実行中の実験。
func Running(ctx context.Context, db *sql.DB, now time.Time) ([]Experiment, error) {
	records, err := query(ctx, db,
		"WHERE stoppedAt IS NULL AND startsAt <= ? AND endsAt > ? ORDER BY id", now, now)
	if err != nil {
		return nil, err
	}
	list := make([]Experiment, len(records))
	for i, r := range records {
		list[i] = r.Experiment
	}
	return list, nil
}

// Recent は新しい順に limit 件の実験。
func Recent(ctx context.Context, db *sql.DB, limit int) ([]Record, error) {
	return query(ctx, db, "ORDER BY id DESC LIMIT ?", limit)
}

// Unreported は、予告なしのくじ（startedBy=schedule）で始め、now の時点で終わっているのに、まだ運営者へ
// 知らせていない実験（古い順）。
func Unreported(ctx context.Context, db *sql.DB, now time.Time) ([]Record, error) {
	return query(ctx, db,
		"WHERE startedBy = 'schedule' AND notifiedAt IS NULL AND (stoppedAt IS NOT NULL OR endsAt <= ?) ORDER BY id", now)
}

// Finished は now の時点で終わっている（止めた・終わる時刻が来た）実験を、新しい順に limit 件。
func Finished(ctx context.Context, db *sql.DB, now time.Time, limit int) ([]Record, error) {
	return query(ctx, db, "WHERE stoppedAt IS NOT NULL OR endsAt <= ? ORDER BY id DESC LIMIT ?", now, limit)
}

func query(ctx context.Context, db *sql.DB, where string, args ...any) ([]Record, error) {
	// #nosec G202 -- where はこのファイルに書いた固定の文だけ。値は args で ? として渡す
	rows, err := db.QueryContext(ctx,
		"SELECT id, kind, route, rate, delayMs, statusCode, startsAt, endsAt, stoppedAt, stoppedBy, startedBy FROM `ChaosExperiment` "+where,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		var r Record
		var kind, startsAt, endsAt string
		var stoppedAt, stoppedBy sql.NullString
		if err := rows.Scan(&r.ID, &kind, &r.Route, &r.Rate, &r.DelayMs, &r.StatusCode,
			&startsAt, &endsAt, &stoppedAt, &stoppedBy, &r.StartedBy); err != nil {
			return nil, err
		}
		r.Kind = Kind(kind)
		if r.StartsAt, err = parseDatetime(startsAt); err != nil {
			return nil, err
		}
		if r.EndsAt, err = parseDatetime(endsAt); err != nil {
			return nil, err
		}
		if stoppedAt.Valid {
			t, err := parseDatetime(stoppedAt.String)
			if err != nil {
				return nil, err
			}
			r.StoppedAt = &t
		}
		r.StoppedBy = stoppedBy.String
		records = append(records, r)
	}
	return records, rows.Err()
}

// parseDatetime は DATETIME(3) の文字列（UTC。internal/database の Config で ParseTime を切っている）を時刻にする。
func parseDatetime(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05.000", s, time.UTC)
}
