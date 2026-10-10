//go:build dbtest

package chaos

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 実験の始める・止めるを、本物の MySQL で確かめる。どの時刻も、ほかの記録と重ならないよう遠い未来にずらす。
func TestChaosDB(t *testing.T) {
	ctx := context.Background()
	fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
	fx.Lock(dbtest.ChaosLock)
	t.Cleanup(func() { fx.Exec("DELETE FROM `ChaosExperiment` WHERE startsAt >= '2100-01-01'") })
	now := time.Date(2100, 1, 1, 3, 0, 0, 0, time.UTC).Add(time.Duration(time.Now().UnixNano() % int64(time.Hour))).Truncate(time.Millisecond)
	experiment := func(start time.Time) Experiment {
		return Experiment{Kind: "db_error", Route: "*", Rate: 0.25, StartsAt: start, EndsAt: start.Add(10 * time.Minute)}
	}

	id, err := Start(ctx, fx.DB, experiment(now), now)
	if err != nil {
		t.Fatal(err)
	}
	if got := fx.Rows("SELECT kind, route, rate, stoppedAt FROM `ChaosExperiment` WHERE id = ?", id); got[0] != `{"kind":"db_error","rate":"0.25","route":"*","stoppedAt":null}` {
		t.Fatalf("row = %v", got)
	}

	t.Run("実行中の実験があれば、次の実験は始めない", func(t *testing.T) {
		later := now.Add(time.Minute)
		if _, err := Start(ctx, fx.DB, experiment(later), later); !errors.Is(err, ErrRunning) {
			t.Fatalf("err = %v, want ErrRunning", err)
		}
	})

	t.Run("止めると、止めた時刻と人を残し、二度目は見つからない", func(t *testing.T) {
		at := now.Add(2 * time.Minute)
		if err := Stop(ctx, fx.DB, id, "admin:u1", at); err != nil {
			t.Fatal(err)
		}
		if got := fx.Rows("SELECT stoppedBy FROM `ChaosExperiment` WHERE id = ?", id); got[0] != `{"stoppedBy":"admin:u1"}` {
			t.Errorf("stoppedBy = %v", got)
		}
		if err := Stop(ctx, fx.DB, id, "admin:u1", at); !errors.Is(err, ErrNotRunning) {
			t.Errorf("err = %v, want ErrNotRunning", err)
		}
	})

	t.Run("終わる時刻を過ぎたら、止めなくても次を始められ、まとめて止められる", func(t *testing.T) {
		start := now.Add(5 * time.Minute)
		second, err := Start(ctx, fx.DB, experiment(start), start)
		if err != nil {
			t.Fatal(err)
		}
		afterEnd := start.Add(10 * time.Minute)
		if err := Stop(ctx, fx.DB, second, "job", afterEnd); !errors.Is(err, ErrNotRunning) {
			t.Fatalf("終わった実験を止めた: %v", err)
		}
		third, err := Start(ctx, fx.DB, experiment(afterEnd), afterEnd)
		if err != nil {
			t.Fatal(err)
		}

		n, err := StopAll(ctx, fx.DB, "job", afterEnd.Add(time.Second))

		if err != nil || n != 1 {
			t.Fatalf("n = %d, err = %v", n, err)
		}
		if got := fx.Rows("SELECT stoppedBy FROM `ChaosExperiment` WHERE id = ?", third); got[0] != `{"stoppedBy":"job"}` {
			t.Errorf("stoppedBy = %v", got)
		}
	})
}
