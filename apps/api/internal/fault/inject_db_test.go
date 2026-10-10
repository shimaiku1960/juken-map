//go:build dbtest

package fault

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 本物の MySQL の接続を包めているかを確かめる。MySQL の接続が fullConn に当たらなくなると（ドライバの更新など）、
// 包まずに返すので障害が黙って起きなくなる。それをここで捕まえる。
func TestWrapConnectorDB(t *testing.T) {
	in, _ := newInjector(t, 0, running(KindDBError, AllRoutes))
	db := dbtest.Open(t, in.WrapConnector)
	reqCtx, _ := requestCtx("GET /api/study-logs")

	t.Run("対象のリクエストの照会・書き込み・トランザクションは失敗する", func(t *testing.T) {
		var n int
		if err := db.QueryRowContext(reqCtx, "SELECT 1").Scan(&n); !errors.Is(err, ErrInjected) {
			t.Errorf("query: err = %v", err)
		}
		if _, err := db.ExecContext(reqCtx, "DO 1"); !errors.Is(err, ErrInjected) {
			t.Errorf("exec: err = %v", err)
		}
		if _, err := db.BeginTx(reqCtx, nil); !errors.Is(err, ErrInjected) {
			t.Errorf("begin: err = %v", err)
		}
	})

	t.Run("リクエストの外（定期の読み込み・cron の外）では失敗しない", func(t *testing.T) {
		var n int
		if err := db.QueryRowContext(context.Background(), "SELECT 1").Scan(&n); err != nil || n != 1 {
			t.Fatalf("n = %d, err = %v", n, err)
		}
	})

	t.Run("終われば、同じ接続のまま元に戻る", func(t *testing.T) {
		in.now = func() time.Time { return t0.Add(time.Hour) }
		var n int
		if err := db.QueryRowContext(reqCtx, "SELECT 1").Scan(&n); err != nil || n != 1 {
			t.Fatalf("n = %d, err = %v", n, err)
		}
	})
}
