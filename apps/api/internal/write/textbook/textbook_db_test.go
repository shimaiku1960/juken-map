//go:build dbtest

package textbook

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// 参考書の操作を、本物の MySQL で確かめる。画面の形（入力チェックの 400・応答の形）は internal/feature/textbooks のテストにある。
func TestTextbookDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// master は総量の候補 metrics（単位と総量、先頭が id の小さい順）を持つ参考書マスターを作る。defaultAt は既定の候補の番号（-1 なら無し）。
	master := func(fx dbtest.Fixture, defaultAt int, metrics ...struct {
		unit  string
		total int64
	}) int64 {
		id := fx.Insert("INSERT INTO TextbookMaster (name, isbn, createdAt, updatedAt) VALUES (?, ?, ?, ?)",
			"マスター-"+dbtest.Hex(6), dbtest.Hex(6), now, now)
		fx.T.Cleanup(func() { fx.Exec("DELETE FROM TextbookMaster WHERE id = ?", id) })
		for i, m := range metrics {
			fx.Exec("INSERT INTO TextbookMasterMetric (masterId, unit, totalAmount, isDefault, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)",
				id, m.unit, m.total, i == defaultAt, now, now)
		}
		return id
	}
	type metric = struct {
		unit  string
		total int64
	}

	t.Run("同じ名前は ErrDuplicate", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		if _, err := Create(ctx, db, user, New{Name: "英単語"}, now); err != nil {
			t.Fatal(err)
		}
		if _, err := Create(ctx, db, user, New{Name: "英単語"}, now); !errors.Is(err, ErrDuplicate) {
			t.Errorf("2回目: %v", err)
		}
	})

	t.Run("マスターからは既定の候補、無ければ先頭の候補を使う", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		withDefault, err := CreateFromMaster(ctx, db, user, master(fx, 1, metric{"page", 300}, metric{"question", 500}), now)
		if err != nil || *withDefault.RangeUnit != "question" || *withDefault.TotalAmount != 500 || withDefault.MasterID == nil {
			t.Errorf("既定あり: %+v, %v", withDefault, err)
		}
		first, err := CreateFromMaster(ctx, db, user, master(fx, -1, metric{"chapter", 12}, metric{"page", 300}), now)
		if err != nil || *first.RangeUnit != "chapter" || *first.TotalAmount != 12 {
			t.Errorf("既定なし: %+v, %v", first, err)
		}
		if _, err := CreateFromMaster(ctx, db, user, master(fx, -1), now); !errors.Is(err, ErrMasterNoMetric) {
			t.Errorf("候補なし: %v", err)
		}
		if _, err := CreateFromMaster(ctx, db, user, -1, now); !errors.Is(err, ErrMasterNotFound) {
			t.Errorf("マスターなし: %v", err)
		}
	})

	t.Run("逆算設定は送った項目だけを変え、他人の参考書は ErrNotFound", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user, other := fx.User(), fx.User()
		created, err := Create(ctx, db, user, New{Name: "数学", Subject: ptr("math")}, now)
		if err != nil {
			t.Fatal(err)
		}
		day := time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
		updated, err := UpdateProgress(ctx, db, user, created.ID,
			Progress{TotalAmount: opt.Of(int64(200)), TargetDate: opt.Of(day)}, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if *updated.TotalAmount != 200 || *updated.TargetDate != "2027-01-31T00:00:00.000Z" || *updated.Subject != "math" || updated.UpdatedAt == created.UpdatedAt {
			t.Errorf("変えた後: %+v", updated)
		}
		cleared, err := UpdateProgress(ctx, db, user, created.ID, Progress{TargetDate: opt.Field[time.Time]{Present: true}}, now)
		if err != nil || cleared.TargetDate != nil {
			t.Errorf("目標日を null に: %+v, %v", cleared, err)
		}
		if _, err := UpdateProgress(ctx, db, other, created.ID, Progress{}, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("他人の参考書: %v", err)
		}
	})
}

func ptr[T any](v T) *T { return &v }
