//go:build dbtest

package account

import (
	"context"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 登録・パスワード・トークン・2段階認証の途中の状態の操作を、本物の MySQL で確かめる。
// 入口からの流れ（登録・確認・再設定・2段階認証）は package main の auth_db_test.go にある。
func TestCredentialDB(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 1, 2, 3, 4_000_000, time.UTC)

	t.Run("登録した利用者は未確認で、確認済みにするのは初めの1回だけ", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := dbtest.Hex(12)
		t.Cleanup(func() { fx.Exec("DELETE FROM `user` WHERE id = ?", id) })
		if err := CreateUserWithPassword(ctx, fx.DB, id, id+"@example.test", "HASH", now); err != nil {
			t.Fatal(err)
		}
		if got := fx.Rows("SELECT u.emailVerified, p.hash FROM `user` u JOIN AuthPassword p ON p.userId = u.id WHERE u.id = ?", id); len(got) != 1 ||
			got[0] != `{"emailVerified":"0","hash":"HASH"}` {
			t.Errorf("作った利用者 = %v", got)
		}
		for i, want := range []bool{true, false} {
			if first, err := MarkEmailVerified(ctx, fx.DB, id, now); err != nil || first != want {
				t.Errorf("%d 回目 = %v %v, want %v", i+1, first, err, want)
			}
		}
	})

	t.Run("パスワードを置き換えると、今の端末以外のセッションとトークン・途中の状態を消す", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		keep, other := dbtest.Hex(16), dbtest.Hex(16)
		for _, s := range []string{keep, other} {
			fx.Exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
				s, dbtest.Hex(16), id, now, now.Add(time.Hour), now)
		}
		if err := IssueToken(ctx, fx.DB, id, "reset-password", []byte(dbtest.Hex(16)), time.Hour, now); err != nil {
			t.Fatal(err)
		}
		if err := CreateMFAChallenge(ctx, fx.DB, id, []byte(dbtest.Hex(16)), time.Minute, now); err != nil {
			t.Fatal(err)
		}

		if err := ReplacePassword(ctx, fx.DB, id, "NEW", keep, now); err != nil {
			t.Fatal(err)
		}

		got := fx.Rows("SELECT (SELECT hash FROM AuthPassword WHERE userId = ?) AS hash,"+
			" (SELECT GROUP_CONCAT(id) FROM AuthSession WHERE userId = ?) AS sessions,"+
			" (SELECT COUNT(*) FROM AuthToken WHERE userId = ?) AS tokens,"+
			" (SELECT COUNT(*) FROM AuthMfaChallenge WHERE userId = ?) AS challenges", id, id, id, id)
		if want := `{"challenges":"0","hash":"NEW","sessions":"` + keep + `","tokens":"0"}`; len(got) != 1 || got[0] != want {
			t.Errorf("置き換えた後 = %v, want %s", got, want)
		}
	})

	t.Run("トークンは用途が合い期限内なら1回だけ使える", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		hash := []byte(dbtest.Hex(16))
		if err := IssueToken(ctx, fx.DB, id, "verify-email", hash, time.Hour, now); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			name, purpose string
			at            time.Time
			want          string
		}{
			{"用途が違う", "reset-password", now, ""},
			{"期限切れ", "verify-email", now.Add(time.Hour), ""},
			{"使える", "verify-email", now, id},
			{"使用済み", "verify-email", now, ""},
		} {
			if got, err := ConsumeToken(ctx, fx.DB, hash, c.purpose, c.at); err != nil || got != c.want {
				t.Errorf("%s: %q %v, want %q", c.name, got, err, c.want)
			}
		}
	})

	t.Run("途中の状態は上限の数まで試せる", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		hash := []byte(dbtest.Hex(16))
		if err := CreateMFAChallenge(ctx, fx.DB, id, hash, time.Minute, now); err != nil {
			t.Fatal(err)
		}
		for i := range 3 {
			want := id
			if i == 2 {
				want = ""
			}
			if got, err := CountMFAChallengeAttempt(ctx, fx.DB, hash, 2, now); err != nil || got != want {
				t.Errorf("%d 回目: %q %v, want %q", i+1, got, err, want)
			}
		}
		if got, err := CountMFAChallengeAttempt(ctx, fx.DB, []byte(dbtest.Hex(16)), 2, now); err != nil || got != "" {
			t.Errorf("無い途中の状態: %q %v", got, err)
		}
	})
}
