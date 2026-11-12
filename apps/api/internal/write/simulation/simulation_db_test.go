//go:build dbtest

package simulation

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// シミュレーションの利用者の印を、本物の MySQL で確かめる。本文の読み方（400）は internal/feature/sim の sim_test.go にある。
func TestSimulationDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// simUser はシミュレーション用のアドレスの利用者を作る。連番は他のテストとぶつからないよう大きく乱数で取る。
	simUser := func(fx dbtest.Fixture) (email string, seq int64) {
		seq = 900_000_000 + rand.Int64N(100_000_000)
		email = "delivered+sim" + dbtest.Hex(4) + "@resend.dev"
		id := "test-sim-" + dbtest.Hex(8)
		fx.Exec("INSERT INTO `user` (id, email, createdAt, updatedAt) VALUES (?, ?, ?, ?)", id, email, now, now)
		fx.T.Cleanup(func() { fx.Exec("DELETE FROM `user` WHERE id = ?", id) })
		return email, seq
	}
	row := func(fx dbtest.Fixture, email string) []string {
		return fx.Rows("SELECT simSeq, simCohort, DATE_FORMAT(simLastActedOn, '%Y-%m-%d') AS lastActedOn,"+
			" DATE_FORMAT(simDormantFrom, '%Y-%m-%d') AS dormantFrom FROM `user` WHERE email = ?", email)
	}
	// tableRow は UserSimulation の行を、row と同じキーで読む。両方が同じなら写せている（JUK-228）。
	tableRow := func(fx dbtest.Fixture, email string) []string {
		return fx.Rows("SELECT s.seq AS simSeq, s.cohort AS simCohort, DATE_FORMAT(s.lastActedOn, '%Y-%m-%d') AS lastActedOn,"+
			" DATE_FORMAT(s.dormantFrom, '%Y-%m-%d') AS dormantFrom FROM UserSimulation s JOIN `user` u ON u.id = s.userId"+
			" WHERE u.email = ?", email)
	}

	t.Run("連番を付け、同じ連番は ErrDuplicate。シミュレーションのアドレスでなければ ErrNotFound", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		email, seq := simUser(fx)
		other, _ := simUser(fx)
		if err := Mark(ctx, db, email, seq, "steady", now); err != nil {
			t.Fatal(err)
		}
		if err := Mark(ctx, db, other, seq, "steady", now); !errors.Is(err, ErrDuplicate) {
			t.Errorf("同じ連番: %v", err)
		}
		if got := tableRow(fx, other); len(got) != 0 {
			t.Errorf("断った利用者が UserSimulation に入った: %v", got)
		}
		real := fx.User() // id@example.test
		if err := Mark(ctx, db, real+"@example.test", seq+1, "steady", now); !errors.Is(err, ErrNotFound) {
			t.Errorf("実ユーザーに連番が付いた: %v", err)
		}
	})

	t.Run("送った日付だけを書き換え、null は消す。連番が無ければ ErrNotFound", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		email, seq := simUser(fx)
		if err := Mark(ctx, db, email, seq, "dropped", now); err != nil {
			t.Fatal(err)
		}
		if err := RecordActivity(ctx, db, seq, Activity{LastActedOn: opt.Of("2026-10-01"), DormantFrom: opt.Of("2026-10-05")}); err != nil {
			t.Fatal(err)
		}
		// 同じ値をもう一度入れても、見つからない扱いにならない（変わった行ではなく当たった行で数える）
		if err := RecordActivity(ctx, db, seq, Activity{LastActedOn: opt.Of("2026-10-01")}); err != nil {
			t.Fatal(err)
		}
		if err := RecordActivity(ctx, db, seq, Activity{DormantFrom: opt.Field[string]{Present: true}}); err != nil {
			t.Fatal(err)
		}
		want := `{"dormantFrom":null,"lastActedOn":"2026-10-01","simCohort":"dropped","simSeq":"` + strconv.FormatInt(seq, 10) + `"}`
		if got := row(fx, email); len(got) != 1 || got[0] != want {
			t.Errorf("行: %v", got)
		}
		if got := tableRow(fx, email); len(got) != 1 || got[0] != want {
			t.Errorf("UserSimulation の行: %v", got)
		}
		if err := RecordActivity(ctx, db, seq+1, Activity{LastActedOn: opt.Of("2026-10-01")}); !errors.Is(err, ErrNotFound) {
			t.Errorf("無い連番: %v", err)
		}
		if err := RecordActivity(ctx, db, seq+1, Activity{}); err != nil {
			t.Errorf("変える項目が無ければ成功にする: %v", err)
		}
	})
	t.Run("付け直すと UserSimulation も付け直す。この段階より前に付いた印は、次に書いたときに写る", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		email, seq := simUser(fx)
		if err := Mark(ctx, db, email, seq, "steady", now); err != nil {
			t.Fatal(err)
		}
		if err := Mark(ctx, db, email, seq+1, "fading", now); err != nil {
			t.Fatal(err)
		}
		want := `{"dormantFrom":null,"lastActedOn":null,"simCohort":"fading","simSeq":"` + strconv.FormatInt(seq+1, 10) + `"}`
		if got := tableRow(fx, email); len(got) != 1 || got[0] != want {
			t.Errorf("付け直した行: %v", got)
		}

		// 古いアプリが user の列だけに書いた利用者（UserSimulation に行が無い）
		old, oldSeq := simUser(fx)
		fx.Exec("UPDATE `user` SET simSeq = ?, simCohort = 'sporadic' WHERE email = ?", oldSeq, old)
		if err := RecordActivity(ctx, db, oldSeq, Activity{LastActedOn: opt.Of("2026-11-01")}); err != nil {
			t.Fatal(err)
		}
		want = `{"dormantFrom":null,"lastActedOn":"2026-11-01","simCohort":"sporadic","simSeq":"` + strconv.FormatInt(oldSeq, 10) + `"}`
		if got := tableRow(fx, old); len(got) != 1 || got[0] != want {
			t.Errorf("あとから写した行: %v", got)
		}
	})
}
