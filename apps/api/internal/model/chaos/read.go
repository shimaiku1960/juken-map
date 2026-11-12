package chaos

import (
	"context"
	"database/sql"
	"time"
)

// 実験の読み取り。

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
