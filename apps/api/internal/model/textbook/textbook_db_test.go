//go:build dbtest

package textbook

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 参考書の操作を、本物の MySQL で確かめる。入力の決まりは input_test.go、応答の形は internal/feature/textbooks のテストにある。
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
		if _, err := Create(ctx, db, user, map[string]any{"name": "英単語"}, now); err != nil {
			t.Fatal(err)
		}
		if _, err := Create(ctx, db, user, map[string]any{"name": "英単語"}, now); !errors.Is(err, ErrDuplicate) {
			t.Errorf("2回目: %v", err)
		}
	})

	t.Run("マスターからは既定の候補、無ければ先頭の候補を使う", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		withDefault, err := Create(ctx, db, user, fromMasterBody(master(fx, 1, metric{"page", 300}, metric{"question", 500})), now)
		if err != nil || *withDefault.RangeUnit != "question" || *withDefault.TotalAmount != 500 || withDefault.MasterID == nil {
			t.Errorf("既定あり: %+v, %v", withDefault, err)
		}
		first, err := Create(ctx, db, user, fromMasterBody(master(fx, -1, metric{"chapter", 12}, metric{"page", 300})), now)
		if err != nil || *first.RangeUnit != "chapter" || *first.TotalAmount != 12 {
			t.Errorf("既定なし: %+v, %v", first, err)
		}
		if _, err := Create(ctx, db, user, fromMasterBody(master(fx, -1)), now); !errors.Is(err, ErrMasterNoMetric) {
			t.Errorf("候補なし: %v", err)
		}
		if _, err := Create(ctx, db, user, fromMasterBody(999999999), now); !errors.Is(err, ErrMasterNotFound) {
			t.Errorf("マスターなし: %v", err)
		}
	})

	t.Run("逆算設定は送った項目だけを変え、他人の参考書は ErrNotFound", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user, other := fx.User(), fx.User()
		created, err := Create(ctx, db, user, map[string]any{"name": "数学", "subject": "math"}, now)
		if err != nil {
			t.Fatal(err)
		}
		updated, err := UpdateProgress(ctx, db, user, created.ID,
			map[string]any{"totalAmount": json.Number("200"), "targetDate": "2027-01-31"}, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if *updated.TotalAmount != 200 || *updated.TargetDate != "2027-01-31T00:00:00.000Z" || *updated.Subject != "math" || updated.UpdatedAt == created.UpdatedAt {
			t.Errorf("変えた後: %+v", updated)
		}
		cleared, err := UpdateProgress(ctx, db, user, created.ID, map[string]any{"targetDate": nil}, now)
		if err != nil || cleared.TargetDate != nil {
			t.Errorf("目標日を null に: %+v, %v", cleared, err)
		}
		if _, err := UpdateProgress(ctx, db, other, created.ID, map[string]any{}, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("他人の参考書: %v", err)
		}
	})
}

// fromMasterBody は参考書マスターから作る本文。
func fromMasterBody(masterID int64) map[string]any {
	return map[string]any{"masterId": json.Number(strconv.FormatInt(masterID, 10))}
}
