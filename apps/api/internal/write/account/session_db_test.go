//go:build dbtest

package account

import (
	"context"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// セッションの操作を、本物の MySQL で確かめる。ログイン・ログアウトの流れは package main の auth_db_test.go、
// 運用のコマンド（revoke・revoke-all・revoke-admins）は incident_db_test.go にある。
func TestSessionDB(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 1, 2, 3, 4_000_000, time.UTC)

	t.Run("作るときに期限と使わないときの期限を書き、期限の切れたセッションを消す", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		expired := dbtest.Hex(16)
		fx.Exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
			expired, dbtest.Hex(16), id, now.Add(-2*time.Hour), now, now.Add(-time.Hour))

		if err := CreateSession(ctx, fx.DB, NewSession{
			TokenHash: []byte(dbtest.Hex(16)), UserID: id, ExpiresAt: now.Add(24 * time.Hour),
			IdleTimeout: time.Hour, MFAVerified: true, IPAddress: "192.0.2.1", UserAgent: "test",
		}, now); err != nil {
			t.Fatal(err)
		}

		got := fx.Rows("SELECT expiresAt, idleTimeoutSeconds, mfaVerifiedAt, ipAddress FROM AuthSession WHERE userId = ?", id)
		want := `{"expiresAt":"2026-10-07 01:02:03.004","idleTimeoutSeconds":"3600","ipAddress":"192.0.2.1","mfaVerifiedAt":"2026-10-06 01:02:03.004"}`
		if len(got) != 1 || got[0] != want {
			t.Errorf("セッション = %v, want [%s]", got, want)
		}
	})

	t.Run("その人のセッションだけを消し、同じトランザクションで記録する", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id, other := fx.User(), fx.User()
		for _, u := range []string{id, id, other} {
			fx.Exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
				dbtest.Hex(16), dbtest.Hex(16), u, now, now.Add(time.Hour), now)
		}
		audit := &OpsAudit{Host: "test-go-" + dbtest.Hex(4)}
		t.Cleanup(func() { fx.Exec("DELETE FROM OpsAuditLog WHERE host = ?", audit.Host) })

		removed, err := RevokeUserSessions(ctx, fx.DB, id, now, audit)

		if err != nil || removed != 2 {
			t.Fatalf("RevokeUserSessions = %d, %v", removed, err)
		}
		if got := fx.Rows("SELECT userId FROM AuthSession WHERE userId IN (?, ?)", id, other); len(got) != 1 || got[0] != `{"userId":"`+other+`"}` {
			t.Errorf("残ったセッション = %v", got)
		}
		if got := fx.Rows("SELECT action, targetId, detail FROM OpsAuditLog WHERE host = ?", audit.Host); len(got) != 1 ||
			got[0] != `{"action":"revoke","detail":"{\"sessionsRemoved\": 2}","targetId":"`+id+`"}` {
			t.Errorf("記録 = %v", got)
		}
	})
}
