//go:build dbtest

package studyrecord

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// 冪等キー付きの作成（CreateLogOnce、学習 2-2・JUK-206）を、本物の MySQL で確かめる。

func TestCreateLogOnceDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	in := LogInput{Date: day, Minutes: 30}
	idem := Idempotency{Key: "key-1", RequestHash: "hash-1"}

	logs := func(t *testing.T, userID string) int {
		return count(t, db, "SELECT COUNT(*) FROM StudyLog WHERE userId = ?", userID)
	}

	t.Run("同じキーで2回送ると1件で、2回目は1回目と同じ結果を返す", func(t *testing.T) {
		user := dbtest.Fixture{T: t, DB: db}.User()
		first, replayed, err := CreateLogOnce(ctx, db, user, idem, in, now)
		if err != nil || replayed || !first.IsFirstStudyLog {
			t.Fatalf("1回目: %+v, replayed=%v, %v", first, replayed, err)
		}
		second, replayed, err := CreateLogOnce(ctx, db, user, idem, in, now.Add(time.Second))
		if err != nil || !replayed {
			t.Fatalf("2回目: replayed=%v, %v", replayed, err)
		}
		if second != first {
			t.Errorf("2回目 = %+v, want %+v", second, first)
		}
		if n := logs(t, user); n != 1 {
			t.Errorf("実績 %d 件, want 1", n)
		}
	})

	t.Run("同じキーで本文が違うと断り、2件目を作らない", func(t *testing.T) {
		user := dbtest.Fixture{T: t, DB: db}.User()
		if _, _, err := CreateLogOnce(ctx, db, user, idem, in, now); err != nil {
			t.Fatal(err)
		}
		other := Idempotency{Key: idem.Key, RequestHash: "hash-2"}
		_, _, err := CreateLogOnce(ctx, db, user, other, LogInput{Date: day, Minutes: 45}, now)
		if !errors.Is(err, ErrIdempotencyKeyReused) {
			t.Errorf("err = %v, want ErrIdempotencyKeyReused", err)
		}
		if n := logs(t, user); n != 1 {
			t.Errorf("実績 %d 件, want 1", n)
		}
	})

	t.Run("キーは利用者ごとに見る", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user, other := fx.User(), fx.User()
		for _, u := range []string{user, other} {
			if _, replayed, err := CreateLogOnce(ctx, db, u, idem, in, now); err != nil || replayed {
				t.Fatalf("%s: replayed=%v, %v", u, replayed, err)
			}
		}
	})

	t.Run("利用者を消すと、実績を通してキーの行も消える（userId に外部キーが無くても残らない）", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		if _, _, err := CreateLogOnce(ctx, db, user, idem, in, now); err != nil {
			t.Fatal(err)
		}
		fx.Exec("DELETE FROM `user` WHERE id = ?", user)
		if n := count(t, db, "SELECT COUNT(*) FROM IdempotencyKey WHERE userId = ?", user); n != 0 {
			t.Errorf("キーの行 %d 件, want 0", n)
		}
	})

	t.Run("1回目が失敗したら、キーの行も残らず、同じキーで記録し直せる", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user, other := fx.User(), fx.User()
		bad := LogInput{Date: day, Minutes: 30,
			TextbookID: opt.Field[int64]{Present: true, Value: ptr(fx.Textbook(other))}}
		if _, _, err := CreateLogOnce(ctx, db, user, idem, bad, now); !errors.Is(err, ErrTextbookNotOwned) {
			t.Fatalf("1回目: %v", err)
		}
		if n := count(t, db, "SELECT COUNT(*) FROM IdempotencyKey WHERE userId = ?", user); n != 0 {
			t.Errorf("キーの行 %d 件, want 0", n)
		}
		if _, replayed, err := CreateLogOnce(ctx, db, user, idem, in, now); err != nil || replayed {
			t.Errorf("2回目: replayed=%v, %v", replayed, err)
		}
	})

	t.Run("同じキーで10本同時に送ると1件で、全員に同じ実績を返す", func(t *testing.T) {
		user := dbtest.Fixture{T: t, DB: db}.User()
		results := runConcurrently(10, func(int) (CreatedLog, bool, error) {
			return CreateLogOnce(ctx, db, user, idem, in, now)
		})
		created := 0
		for i, r := range results {
			if r.err != nil {
				t.Fatalf("%d本目: %v", i, r.err)
			}
			if !r.replayed {
				created++
			}
			if r.log.ID != results[0].log.ID || !r.log.IsFirstStudyLog {
				t.Errorf("%d本目 = %+v, want id %d・初回", i, r.log, results[0].log.ID)
			}
		}
		if created != 1 {
			t.Errorf("新しく作ったのは %d 本, want 1", created)
		}
		if n := logs(t, user); n != 1 {
			t.Errorf("実績 %d 件, want 1", n)
		}
	})

	t.Run("同じ利用者が別々のキーで10本同時に送ると10件", func(t *testing.T) {
		user := dbtest.Fixture{T: t, DB: db}.User()
		results := runConcurrently(10, func(i int) (CreatedLog, bool, error) {
			k := Idempotency{Key: fmt.Sprintf("key-%d", i), RequestHash: "hash-1"}
			return CreateLogOnce(ctx, db, user, k, in, now)
		})
		for i, r := range results {
			if r.err != nil {
				t.Errorf("%d本目: %v", i, r.err)
			}
		}
		if n := logs(t, user); n != 10 {
			t.Errorf("実績 %d 件, want 10", n)
		}
	})
}

type onceResult struct {
	log      CreatedLog
	replayed bool
	err      error
}

// runConcurrently は fn を n 本同時に走らせ、全部終わるのを待って結果を返す。
func runConcurrently(n int, fn func(i int) (CreatedLog, bool, error)) []onceResult {
	results := make([]onceResult, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // 全員がそろってから一斉に始める
			log, replayed, err := fn(i)
			results[i] = onceResult{log, replayed, err}
		}()
	}
	close(start)
	wg.Wait()
	return results
}
