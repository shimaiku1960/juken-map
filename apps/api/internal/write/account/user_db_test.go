//go:build dbtest

package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 利用者の行の操作（外部ログインの連携・削除・権限）を、本物の MySQL で確かめる。入口からの流れは
// internal/feature/auth の auth_db_test.go と、internal/app の admin_db_test.go、internal/feature/ops の incident_db_test.go にある。
func TestUserDB(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 1, 2, 3, 4_000_000, time.UTC)

	t.Run("外部ログインは、いなければ作り、確認済みなら結びつけ、未確認ならログインの手段を消して取り戻す", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		verified, unverified := fx.User(), fx.User()
		fx.Exec("UPDATE `user` SET emailVerified = true WHERE id = ?", verified)
		fx.Exec("INSERT INTO AuthPassword (userId, hash, updatedAt) VALUES (?, 'HASH', ?)", unverified, now)
		fx.Exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
			dbtest.Hex(16), dbtest.Hex(16), unverified, now, now.Add(time.Hour), now)
		newEmail := dbtest.Hex(8) + "@example.test"
		t.Cleanup(func() { fx.Exec("DELETE FROM `user` WHERE email = ?", newEmail) })

		for _, c := range []struct {
			email  string
			wantID string
			want   LinkEvent
		}{
			{newEmail, "", LinkCreated},
			{verified + "@example.test", verified, LinkLinked},
			{unverified + "@example.test", unverified, LinkClaimedUnverified},
		} {
			id, event, err := LinkOAuthIdentity(ctx, fx.DB, Identity{Provider: "google", Subject: dbtest.Hex(8), Email: c.email, Name: "名前"}, now)
			if err != nil || event != c.want || (c.wantID != "" && id != c.wantID) {
				t.Errorf("%s: %q %q %v, want %q", c.email, id, event, err, c.want)
			}
		}
		got := fx.Rows("SELECT u.emailVerified,"+
			" (SELECT COUNT(*) FROM AuthPassword WHERE userId = u.id) AS passwords,"+
			" (SELECT COUNT(*) FROM AuthSession WHERE userId = u.id) AS sessions,"+
			" (SELECT COUNT(*) FROM AuthIdentity WHERE userId = u.id) AS identities FROM `user` u WHERE u.id = ?", unverified)
		if want := `{"emailVerified":"1","identities":"1","passwords":"0","sessions":"0"}`; len(got) != 1 || got[0] != want {
			t.Errorf("取り戻した利用者 = %v, want %s", got, want)
		}
	})

	t.Run("削除すると本人のデータの数を返して消す。いなければ ErrNotFound", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		fx.Textbook(id)
		fx.StudyLog(id)
		fx.StudyLog(id)

		removed, err := DeleteUser(ctx, fx.DB, id)

		if err != nil || removed != (Removed{StudyLogs: 2, Textbooks: 1}) {
			t.Fatalf("DeleteUser = %+v, %v", removed, err)
		}
		if _, err := DeleteUser(ctx, fx.DB, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("2回目 = %v, want ErrNotFound", err)
		}
	})

	t.Run("確認前の人は管理者にせず、付け替えたらセッションを消して記録する", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		fx.Exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
			dbtest.Hex(16), dbtest.Hex(16), id, now, now.Add(time.Hour), now)
		audit := &OpsAudit{Host: "test-go-" + dbtest.Hex(4)}
		t.Cleanup(func() { fx.Exec("DELETE FROM OpsAuditLog WHERE host = ?", audit.Host) })

		if _, _, err := SetRole(ctx, fx.DB, id, "admin", now, audit); !errors.Is(err, ErrUnverified) {
			t.Fatalf("確認前 = %v, want ErrUnverified", err)
		}
		fx.Exec("UPDATE `user` SET emailVerified = true WHERE id = ?", id)
		previous, removed, err := SetRole(ctx, fx.DB, id, "admin", now, audit)

		if err != nil || previous != "user" || removed != 1 {
			t.Fatalf("SetRole = %q %d %v", previous, removed, err)
		}
		got := fx.Rows("SELECT u.role, (SELECT COUNT(*) FROM OpsAuditLog WHERE host = ? AND action = 'set-role' AND targetId = u.id) AS audit"+
			" FROM `user` u WHERE u.id = ?", audit.Host, id)
		if want := `{"audit":"1","role":"admin"}`; len(got) != 1 || got[0] != want {
			t.Errorf("付け替えた後 = %v, want %s", got, want)
		}
	})
}
