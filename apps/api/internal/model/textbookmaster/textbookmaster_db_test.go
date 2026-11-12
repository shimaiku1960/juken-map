//go:build dbtest

package textbookmaster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 参考書マスターの操作を、本物の MySQL で確かめる。管理画面の入口を通した流れ（文言・監査ログ）は internal/app の TestAdminMastersDB にある。
func TestTextbookMasterDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	fx := dbtest.Fixture{T: t, DB: db}
	now := time.Now().UTC().Truncate(time.Millisecond)

	// ISBN は13桁の数字（入力の決まり）。重ならないよう時刻から作る。internal/app の DB テストは 979 で始めるので 978 にする。
	isbn := fmt.Sprintf("978%010d", time.Now().UnixNano()%1e10)
	metric := func(unit string, total int, isDefault bool) map[string]any {
		return map[string]any{"unit": unit, "totalAmount": json.Number(fmt.Sprint(total)), "isDefault": isDefault}
	}
	body := map[string]any{"name": " 英単語 ", "publisher": "", "isbn": isbn, "metrics": []any{metric("page", 300, false), metric("number", 1900, true)}}
	created, err := Create(ctx, db, body, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fx.Exec("DELETE FROM TextbookMaster WHERE id = ?", created.ID) })
	if created.Name != "英単語" || created.Publisher != nil {
		t.Errorf("名前は削り、空の出版社は null: %+v", created)
	}
	if want := (Metric{IsDefault: true, Unit: "number", TotalAmount: 1900}); len(created.Metrics) != 2 || created.Metrics[1] != want {
		t.Errorf("総量の候補: %+v", created.Metrics)
	}
	if _, err := Create(ctx, db, body, now); !errors.Is(err, ErrDuplicate) {
		t.Errorf("同じ ISBN: %v", err)
	}
	var issue *validation.Issue
	if _, err := Create(ctx, db, map[string]any{"name": "x"}, now); !errors.As(err, &issue) {
		t.Errorf("形の違う本文: %v", err)
	}

	got, err := List(ctx, db, map[string][]string{"q": {" " + isbn + " "}})
	if err != nil || len(got) != 1 || got[0].ID != created.ID || len(got[0].Metrics) != 2 {
		t.Errorf("一覧を ISBN で絞る: %+v, %v", got, err)
	}
	if _, err := List(ctx, db, map[string][]string{"q": {"a", "b"}}); !errors.As(err, &issue) {
		t.Errorf("q が2つ: %v", err)
	}

	body["metrics"] = []any{metric("chapter", 12, true)}
	change, err := Update(ctx, db, created.ID, body, now)
	if err != nil || len(change.Before.Metrics) != 2 || len(change.After.Metrics) != 1 || change.After.Metrics[0].Unit != "chapter" {
		t.Errorf("書き換え: %+v, %v", change, err)
	}
	if _, err := Update(ctx, db, -1, body, now); !errors.Is(err, ErrNotFound) {
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
