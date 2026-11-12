// Package chaos は障害注入の実験（ChaosExperiment、JUK-173）のモデル。実験の種類と中身、入力の決まり（input.go）、
// 読み取り（read.go）と書き込み（このファイル）を持ち、HTTP は知らない（docs/architecture.md「モデル・コントローラー・ビュー」、JUK-226）。
// 障害を起こすのは internal/fault（アプリの層）と internal/hostfault（ホストの層）。
package chaos

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrRunning は、ほかの実験がまだ実行中であること。気づくまでの時間を1つずつ測れるよう、同時には1つだけにする。
// Error() は利用者に見せる文言。
var ErrRunning = errors.New("ほかの実験が実行中です")

// 止めた人（stoppedBy）。管理画面は StoppedByAdmin に利用者の id を付ける。
const (
	StoppedByAdmin  = "admin:"
	StoppedByJob    = "job"    // 機械の入口（/api/chaos/）
	StoppedByDeploy = "deploy" // デプロイの前後（`/api chaos stop`、.github/scripts/deploy-ec2.sh）
	StoppedByFailed = "failed" // 予告なしのくじが、ホストで障害を起こせなかった（JUK-174）
)

// 始めた入口（startedBy）。終わったら運営者へ知らせるのは StartedBySchedule の行だけ。
const (
	StartedByJob      = "job"      // 機械の入口（/api/chaos/）
	StartedBySchedule = "schedule" // 予告なしのくじ（feature/chaos/schedule.go）
)

// NewExperiment は新しく始める実験。
type NewExperiment struct {
	Kind       Kind
	Route      string
	Rate       float64
	DelayMs    int
	StatusCode int
	StartsAt   time.Time
	EndsAt     time.Time
	StartedBy  string // StartedBy…。空なら StartedByJob
}

// Start は実験を記録して、その id を返す。now の時点で実行中の実験があれば ErrRunning。
//
// 「実行中が無ければ入れる」を1つの文にする。2つ同時に来ると、片方は相手の行の範囲のロックで待つか
// デッドロックで失敗し、両方は入らない。
func Start(ctx context.Context, db *sql.DB, e NewExperiment, now time.Time) (int64, error) {
	startedBy := e.StartedBy
	if startedBy == "" {
		startedBy = StartedByJob
	}
	res, err := db.ExecContext(ctx,
		"INSERT INTO `ChaosExperiment` (kind, route, rate, delayMs, statusCode, startsAt, endsAt, startedBy) "+
			"SELECT ?, ?, ?, ?, ?, ?, ?, ? FROM DUAL WHERE NOT EXISTS "+
			"(SELECT 1 FROM `ChaosExperiment` WHERE stoppedAt IS NULL AND endsAt > ?)",
		e.Kind, e.Route, e.Rate, e.DelayMs, e.StatusCode, e.StartsAt, e.EndsAt, startedBy, now)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, ErrRunning
	}
	return res.LastInsertId()
}

// StartFromBody は機械の入口（POST /api/chaos/experiments）の本文を確かめて、実験を今から始め、その id を返す。
// routes は障害注入の対象にしてよいルート。本文が決まりに合わなければ InputError、実行中の実験があれば ErrRunning。
func StartFromBody(ctx context.Context, db *sql.DB, body any, routes []string, now time.Time) (int64, error) {
	spec, err := decodeSpec(body)
	if err != nil {
		return 0, err
	}
	if err := spec.Validate(routes); err != nil {
		return 0, err
	}
	return Start(ctx, db, spec.StartingAt(now, StartedByJob), now)
}

// StopAll は実行中の実験を全部止めて、止めた数を返す。by は止めた人（StoppedBy…）。
func StopAll(ctx context.Context, db *sql.DB, by string, now time.Time) (int64, error) {
	res, err := db.ExecContext(ctx,
		"UPDATE `ChaosExperiment` SET stoppedAt = ?, stoppedBy = ? WHERE stoppedAt IS NULL AND endsAt > ?",
		now, by, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Stop は id の実験が実行中なら止める。by は止めた人（StoppedBy…）。
func Stop(ctx context.Context, db *sql.DB, id int64, by string, now time.Time) error {
	_, err := db.ExecContext(ctx,
		"UPDATE `ChaosExperiment` SET stoppedAt = ?, stoppedBy = ? WHERE id = ? AND stoppedAt IS NULL AND endsAt > ?",
		now, by, id, now)
	return err
}

// ClaimNotify は id の実験を「知らせた」にし、自分が入れたら true を返す。blue/green の切り替えで2つのプロセスが
// 同じ行を拾っても、true になるのは片方だけなので、知らせは1通になる（入れてから送るので、送るのに失敗したら届かない）。
func ClaimNotify(ctx context.Context, db *sql.DB, id int64, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx,
		"UPDATE `ChaosExperiment` SET notifiedAt = ? WHERE id = ? AND notifiedAt IS NULL", now, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
