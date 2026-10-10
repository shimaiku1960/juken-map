//go:build dbtest

package chaos

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
	"github.com/shimaiku1960/juken-map/apps/api/internal/hostfault"
	writechaos "github.com/shimaiku1960/juken-map/apps/api/internal/write/chaos"
)

// fakeHost はホストの層の障害を起こした・戻した回数を数える。
type fakeHost struct {
	startErr        error
	starts, reverts int
}

func (h *fakeHost) Start(context.Context, fault.Experiment) error {
	h.starts++
	return h.startErr
}

func (h *fakeHost) Revert(context.Context) error {
	h.reverts++
	return nil
}

// くじで当たった実験を始め、終わったら知らせるまでを、本物の MySQL で確かめる。時刻はほかの記録と重ならないよう遠い未来にずらす。
func TestSchedulerRunDB(t *testing.T) {
	ctx := context.Background()
	fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
	fx.Lock(dbtest.ChaosLock)
	t.Cleanup(func() { fx.Exec("DELETE FROM `ChaosExperiment` WHERE startsAt >= '2100-01-01'") })
	// 前に残った知らせていない記録を、ここでの知らせに混ぜない。
	fx.Exec("UPDATE `ChaosExperiment` SET notifiedAt = startsAt WHERE notifiedAt IS NULL")
	base := time.Date(2100, 1, 1, 3, 0, 0, 0, time.UTC).Add(time.Duration(time.Now().UnixNano() % int64(time.Hour))).Truncate(time.Millisecond)

	newScheduler := func(now *time.Time, mails *[]string, host hostfault.Runner) *Scheduler {
		return &Scheduler{
			DB: fx.DB, Injector: fault.New(), Routes: func() []string { return testRoutes },
			Notify: func(_ context.Context, _, body string) error {
				*mails = append(*mails, body)
				return nil
			},
			Host: host,
			Rand: rand.New(rand.NewPCG(1, 2)),
			Now:  func() time.Time { return *now },
		}
	}
	spec := fault.Spec{Kind: fault.KindDBError, Route: fault.AllRoutes, Rate: 0.5, Duration: 10 * time.Minute}

	t.Run("終わる時刻が来たら、どのプロセスからでも一度だけ知らせる", func(t *testing.T) {
		now := base
		var mails []string
		a, b := newScheduler(&now, &mails, nil), newScheduler(&now, &mails, nil)
		a.run(ctx, spec)
		a.report(ctx)
		if len(mails) != 0 {
			t.Fatalf("終わる前に知らせた: %v", mails)
		}
		now = base.Add(spec.Duration)
		a.report(ctx)
		b.report(ctx)
		if len(mails) != 1 || !strings.Contains(mails[0], "終わる時刻が来て戻った") || !strings.Contains(mails[0], "DB の失敗") {
			t.Errorf("mails = %v", mails)
		}
	})

	t.Run("ホストの障害をデプロイで止めたら、ホストを戻して止め方を知らせる", func(t *testing.T) {
		now := base.Add(time.Hour)
		var mails []string
		host := &fakeHost{}
		s := newScheduler(&now, &mails, host)
		s.run(ctx, hostfault.Spec(hostfault.KindCPU, 80, 10*time.Minute))
		if host.starts != 1 {
			t.Fatalf("starts = %d", host.starts)
		}
		now = now.Add(3 * time.Minute)
		if _, err := writechaos.StopAll(ctx, fx.DB, writechaos.StoppedByDeploy, now); err != nil {
			t.Fatal(err)
		}
		s.report(ctx)
		if host.reverts != 1 || len(mails) != 1 || !strings.Contains(mails[0], "（デプロイで止めた）") || !strings.Contains(mails[0], "CPU の圧迫") {
			t.Errorf("reverts = %d, mails = %v", host.reverts, mails)
		}
	})

	t.Run("ホストで起こせなければ、すぐ止めて知らせる", func(t *testing.T) {
		now := base.Add(2 * time.Hour)
		var mails []string
		host := &fakeHost{startErr: errors.New("ssm")}
		s := newScheduler(&now, &mails, host)
		s.run(ctx, hostfault.Spec(hostfault.KindDBDelay, 300, 10*time.Minute))
		s.report(ctx)
		if host.reverts != 0 || len(mails) != 1 || !strings.Contains(mails[0], "ホストで起こせず") {
			t.Errorf("reverts = %d, mails = %v", host.reverts, mails)
		}
	})

	t.Run("ほかの実験が実行中なら、始めず知らせもしない", func(t *testing.T) {
		start := base.Add(3 * time.Hour)
		if _, err := writechaos.Start(ctx, fx.DB, writechaos.Experiment{Kind: "db_error", Route: "*", Rate: 0.1,
			StartsAt: start, EndsAt: start.Add(time.Hour)}, start); err != nil {
			t.Fatal(err)
		}
		now := start.Add(time.Minute)
		var mails []string
		host := &fakeHost{}
		s := newScheduler(&now, &mails, host)
		s.run(ctx, hostfault.Spec(hostfault.KindDisk, 90, 10*time.Minute))
		s.report(ctx)
		if host.starts != 0 || len(mails) != 0 {
			t.Errorf("starts = %d, mails = %v", host.starts, mails)
		}
	})
}
