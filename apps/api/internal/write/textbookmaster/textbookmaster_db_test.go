//go:build dbtest

package textbookmaster

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 参考書マスターの操作を、本物の MySQL で確かめる。管理画面の入口を通した流れ（文言・監査ログ）は internal/app の TestAdminMastersDB にある。
func TestTextbookMasterDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	fx := dbtest.Fixture{T: t, DB: db}
	now := time.Now().UTC().Truncate(time.Millisecond)

	in := Input{Name: "英単語", Isbn: dbtest.Hex(6), Metrics: []Metric{{Unit: "page", TotalAmount: 300}, {IsDefault: true, Unit: "number", TotalAmount: 1900}}}
	created, err := Create(ctx, db, in, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fx.Exec("DELETE FROM TextbookMaster WHERE id = ?", created.ID) })
	if len(created.Metrics) != 2 || created.Metrics[1] != in.Metrics[1] {
		t.Errorf("総量の候補: %+v", created.Metrics)
	}
	if _, err := Create(ctx, db, in, now); !errors.Is(err, ErrDuplicate) {
		t.Errorf("同じ ISBN: %v", err)
	}

	in.Metrics = []Metric{{IsDefault: true, Unit: "chapter", TotalAmount: 12}}
	change, err := Update(ctx, db, created.ID, in, now)
	if err != nil || len(change.Before.Metrics) != 2 || len(change.After.Metrics) != 1 || change.After.Metrics[0].Unit != "chapter" {
		t.Errorf("書き換え: %+v, %v", change, err)
	}
	if _, err := Update(ctx, db, -1, in, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("無い参考書マスター: %v", err)
	}

	user := fx.User()
	fx.Exec("INSERT INTO Textbook (userId, name, masterId, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)", user, "英単語", created.ID, now, now)
	var inUse *InUseError
	if _, err := Delete(ctx, db, created.ID); !errors.As(err, &inUse) || inUse.Count != 1 {
		t.Errorf("使われている参考書マスター: %v", err)
	}
	fx.Exec("DELETE FROM Textbook WHERE userId = ?", user)
	if deleted, err := Delete(ctx, db, created.ID); err != nil || deleted.ID != created.ID {
		t.Errorf("消す: %+v, %v", deleted, err)
	}
	if n := fx.Rows("SELECT id FROM TextbookMasterMetric WHERE masterId = ?", created.ID); len(n) != 0 {
		t.Errorf("総量の候補が残っている: %v", n)
	}
}
