//go:build learn

// 学習 1-4（JUK-197）の実験。接続プール（database/sql）の上限と、トランザクションが接続を借りっぱなしにする
// ことの関係を見る。`-tags learn` を付けたときだけ動き、CI では動かない。表には触れず、SELECT 1 だけを流す。
//
//	LEARN_DATABASE_URL=mysql://juken:jukenpassword@localhost:3306/juken_map \
//	  go test -tags learn -run TestLearnPool -v ./internal/database/
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

// openLearnDB は localhost の DB にだけ繋ぎ、プールの上限を maxOpen 本にする。
func openLearnDB(t *testing.T, maxOpen int) *sql.DB {
	t.Helper()
	raw := os.Getenv("LEARN_DATABASE_URL")
	if raw == "" {
		t.Skip("LEARN_DATABASE_URL が無いので飛ばす")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if h := u.Hostname(); h != "localhost" && h != "127.0.0.1" {
		t.Fatalf("手元の DB 以外には繋がない（host=%s）", h)
	}
	db, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(maxOpen)
	t.Cleanup(func() { db.Close() })
	return db
}

// 実験1：上限1本のプールで、トランザクションの中から tx ではなく db で SQL を流す。
func TestLearnPoolBorrowInsideTx(t *testing.T) {
	db := openLearnDB(t, 1)
	ctx := context.Background()

	// トランザクションを始める。ここでプールの1本を、Commit か Rollback まで借りっぱなしにする。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	t.Logf("BeginTx のあと: 使用中=%d 空き=%d", db.Stats().InUse, db.Stats().Idle)

	// うっかり tx ではなく db で読む。2秒で諦める。
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	var one int
	err = db.QueryRowContext(waitCtx, `SELECT 1`).Scan(&one)
	elapsed := time.Since(start)

	stats := db.Stats()
	t.Logf("db で SELECT 1: err=%v かかった時間=%v", err, elapsed.Round(10*time.Millisecond))
	t.Logf("プールの数字: 待った回数=%d 待った時間の合計=%v", stats.WaitCount, stats.WaitDuration.Round(10*time.Millisecond))
}

// 実験2：上限15本（本番と同じ）のプールで、N 人が同時に「BeginTx → db で SELECT 1 → Commit」をする。
// 1人ずつ3秒で諦める。N を変えて、成功した人数・失敗した人数・全体の時間を比べる。
func TestLearnPoolManyBorrowInsideTx(t *testing.T) {
	for _, n := range []int{1, 5, 14, 15, 16} {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			db := openLearnDB(t, 15)

			var mu sync.Mutex
			ok, failed := 0, 0
			var wg sync.WaitGroup
			start := time.Now()
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := borrowInsideTx(db)
					mu.Lock()
					defer mu.Unlock()
					if err != nil {
						failed++
					} else {
						ok++
					}
				}()
			}
			wg.Wait()

			t.Logf("N=%2d: 成功=%2d 失敗=%2d 全体の時間=%v 待った回数=%d",
				n, ok, failed, time.Since(start).Round(10*time.Millisecond), db.Stats().WaitCount)
		})
	}
}

// borrowInsideTx は1人ぶんの仕事。トランザクションで1本借りたまま、db でもう1本借りに行く。
func borrowInsideTx(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// トランザクションの中で何か仕事をしている時間。全員が BeginTx を済ませるのを待つ役目もある。
	time.Sleep(200 * time.Millisecond)

	var one int
	if err := db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return err
	}
	return tx.Commit()
}

// 実験3：50人が3秒間、20ms かかる SQL（SELECT SLEEP(0.02)）を休まず流し続ける。プールの上限だけを変えて、
// 3秒で終わった本数と、1本あたりの時間（接続を待った時間を含む）の中央値・p95 を比べる。
func TestLearnPoolSizeLatency(t *testing.T) {
	for _, size := range []int{2, 5, 15, 50} {
		t.Run(fmt.Sprintf("上限=%d", size), func(t *testing.T) {
			db := openLearnDB(t, size)
			lat := runLoad(db, 50, 3*time.Second, `SELECT SLEEP(0.02)`)
			stats := db.Stats()
			t.Logf("上限=%2d: 3秒で終わった本数=%5d  中央値=%v  p95=%v  待った回数=%d",
				size, len(lat), percentile(lat, 50), percentile(lat, 95), stats.WaitCount)
		})
	}
}

// runLoad は workers 人で d のあいだ query を流し続け、1本ずつかかった時間を返す。
func runLoad(db *sql.DB, workers int, d time.Duration, query string) []time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	var mu sync.Mutex
	var lat []time.Duration
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				start := time.Now()
				// 時間切れで打ち切られた最後の1本は数えない。
				if _, err := db.ExecContext(ctx, query); err != nil {
					return
				}
				mu.Lock()
				lat = append(lat, time.Since(start))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return lat
}

// percentile は小さい順に並べて p% の位置の値を返す。
func percentile(lat []time.Duration, p int) time.Duration {
	if len(lat) == 0 {
		return 0
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	return lat[(len(lat)-1)*p/100].Round(time.Millisecond)
}

// 実験4：実験3と同じ形で、SQL を MySQL の CPU を使うもの（SHA2 を5万回、手元で約22ms）に変える。
// 1本の時間を「プールで接続を待った時間」と「それ以外（MySQL の中）」に分けて見る。
func TestLearnPoolSizeCPU(t *testing.T) {
	for _, size := range []int{2, 5, 10, 15, 50} {
		t.Run(fmt.Sprintf("上限=%d", size), func(t *testing.T) {
			db := openLearnDB(t, size)
			lat := runLoad(db, 50, 3*time.Second, `SELECT BENCHMARK(50000, SHA2('juken', 256))`)
			stats := db.Stats()
			var waitPerQuery time.Duration
			if len(lat) > 0 {
				waitPerQuery = (stats.WaitDuration / time.Duration(len(lat))).Round(time.Millisecond)
			}
			t.Logf("上限=%2d: 3秒で終わった本数=%4d  中央値=%v  p95=%v  うちプールで待った時間(平均)=%v",
				size, len(lat), percentile(lat, 50), percentile(lat, 95), waitPerQuery)
		})
	}
}
