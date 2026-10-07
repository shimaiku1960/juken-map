//go:build dbtest

package goal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// 志望校の操作を、本物の MySQL で確かめる。画面の形（入力チェックの 400・応答の形）は internal/feature/goals のテストにある。
func TestGoalDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	t.Run("同じ学部は ErrDuplicate、ステータスの既定は decided", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		faculty := fx.University() // 片付けは逆順なので、学部は利用者（志望校ごと消える）より先に作る
		user := fx.User()
		id, err := Create(ctx, db, user, faculty, nil, now)
		if err != nil {
			t.Fatal(err)
		}
		if g, err := find(ctx, db, id, user, ""); err != nil || g.Status != "decided" || g.FacultyID != faculty {
			t.Errorf("登録した行: %+v, %v", g, err)
		}
		if _, err := Create(ctx, db, user, faculty, nil, now); !errors.Is(err, ErrDuplicate) {
			t.Errorf("2回目: %v", err)
		}
	})

	t.Run("第一志望は1校だけ残る", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		a, b := fx.University(), fx.University()
		user := fx.User()
		first, second := fx.FinalGoal(user, a), fx.FinalGoal(user, b)
		for _, id := range []int64{first, second} {
			if err := ApplyPatch(ctx, db, user, id, Patch{IsFirstChoice: opt.Of(true)}); err != nil {
				t.Fatal(err)
			}
		}
		g1, _ := find(ctx, db, first, user, "")
		g2, _ := find(ctx, db, second, user, "")
		if g1.IsFirstChoice || !g2.IsFirstChoice {
			t.Errorf("第一志望: 1校目 %v・2校目 %v", g1.IsFirstChoice, g2.IsFirstChoice)
		}
	})

	t.Run("差し替え・メモ・削除は他人の志望校なら ErrNotFound", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		faculty, before := fx.University(), fx.University()
		user, other := fx.User(), fx.User()
		id := fx.FinalGoal(user, before)
		if _, err := ReplaceFaculty(ctx, db, other, id, &faculty); !errors.Is(err, ErrNotFound) {
			t.Errorf("差し替え: %v", err)
		}
		if err := ApplyPatch(ctx, db, other, id, Patch{}); !errors.Is(err, ErrNotFound) {
			t.Errorf("メモ: %v", err)
		}
		if err := Delete(ctx, db, other, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("削除: %v", err)
		}
		replaced, err := ReplaceFaculty(ctx, db, user, id, &faculty)
		if err != nil || replaced.FacultyID != faculty {
			t.Errorf("自分の差し替え: %+v, %v", replaced, err)
		}
		if err := Delete(ctx, db, user, id); err != nil {
			t.Errorf("自分の削除: %v", err)
		}
	})
}
