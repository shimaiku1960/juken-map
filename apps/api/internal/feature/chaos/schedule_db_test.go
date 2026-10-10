//go:build dbtest

package chaos

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
	writechaos "github.com/shimaiku1960/juken-map/apps/api/internal/write/chaos"
)

// くじで当たった実験を始め、終わったら知らせるまでを、本物の MySQL で確かめる。時刻はほかの記録と重ならないよう遠い未来にずらす。
func TestSchedulerRunDB(t *testing.T) {
	ctx := context.Background()
	fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
	fx.Lock(dbtest.ChaosLock)
	t.Cleanup(func() { fx.Exec("DELETE FROM `ChaosExperiment` WHERE startsAt >= '2100-01-01'") })
	base := time.Date(2100, 1, 1, 3, 0, 0, 0, time.UTC).Add(time.Duration(time.Now().UnixNano() % int64(time.Hour))).Truncate(time.Millisecond)

	newScheduler := func(clock *atomic.Pointer[time.Time], mails chan<- string) *Scheduler {
		return &Scheduler{
			DB: fx.DB, Injector: fault.New(), Routes: func() []string { return testRoutes },
			Notify: func(_ context.Context, _, body string) error {
				mails <- body
				return nil
			},
			Rand:       rand.New(rand.NewPCG(1, 2)),
			Now:        func() time.Time { return *clock.Load() },
			watchEvery: time.Millisecond,
		}
	}
	spec := fault.Spec{Kind: fault.KindDBError, Route: fault.AllRoutes, Rate: 0.5, Duration: 10 * time.Minute}

	t.Run("終わる時刻が来たら、終わったことを知らせる", func(t *testing.T) {
		var clock atomic.Pointer[time.Time]
		clock.Store(&base)
		mails := make(chan string, 1)
		s := newScheduler(&clock, mails)
		done := make(chan struct{})
		go func() { s.run(ctx, spec); close(done) }()

		waitRows(t, fx, base)
		ended := base.Add(spec.Duration)
		clock.Store(&ended)
		<-done
		if body := <-mails; !strings.Contains(body, "終わる時刻が来て戻った") || !strings.Contains(body, "DB の失敗") {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("デプロイで止めたら、止めた時刻と止め方を知らせる", func(t *testing.T) {
		start := base.Add(time.Hour)
		var clock atomic.Pointer[time.Time]
		clock.Store(&start)
		mails := make(chan string, 1)
		s := newScheduler(&clock, mails)
		done := make(chan struct{})
		go func() { s.run(ctx, spec); close(done) }()

		waitRows(t, fx, start)
		at := start.Add(3 * time.Minute)
		if _, err := writechaos.StopAll(ctx, fx.DB, writechaos.StoppedByDeploy, at); err != nil {
			t.Fatal(err)
		}
		clock.Store(&at)
		<-done
		if body := <-mails; !strings.Contains(body, "（デプロイで止めた）") {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("ほかの実験が実行中なら、始めず知らせもしない", func(t *testing.T) {
		start := base.Add(2 * time.Hour)
		if _, err := writechaos.Start(ctx, fx.DB, writechaos.Experiment{Kind: "db_error", Route: "*", Rate: 0.1,
			StartsAt: start, EndsAt: start.Add(time.Hour)}, start); err != nil {
			t.Fatal(err)
		}
		later := start.Add(time.Minute)
		var clock atomic.Pointer[time.Time]
		clock.Store(&later)
		mails := make(chan string, 1)
		newScheduler(&clock, mails).run(ctx, spec)
		if len(mails) != 0 {
			t.Errorf("知らせた: %s", <-mails)
		}
	})
}

// waitRows は startsAt が at の実験が入るまで待つ。
func waitRows(t *testing.T, fx dbtest.Fixture, at time.Time) {
	t.Helper()
	for range 500 {
		if len(fx.Rows("SELECT id FROM `ChaosExperiment` WHERE startsAt = ?", at)) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("実験が始まらない")
}
