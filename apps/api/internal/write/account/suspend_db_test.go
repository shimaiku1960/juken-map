//go:build dbtest

package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 利用停止・解除の操作を、本物の MySQL で確かめる。管理画面と運用のコマンドを通した確認は
// internal/app の admin_db_test.go、internal/feature/ops の incident_db_test.go にある。
func TestSuspendDB(t *testing.T) {
	ctx := context.Background()

	// setup は利用者を作り、セッションを sessions 個持たせる。記録は host で見分けて、テストの終わりに消す。
	setup := func(t *testing.T, sessions int) (dbtest.Fixture, string, *OpsAudit) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		now := time.Now().UTC()
		for range sessions { // tokenHash は BINARY(32)。32文字の16進をそのまま入れる
			fx.Exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
				dbtest.Hex(16), dbtest.Hex(16), id, now, now.Add(time.Hour), now)
		}
		audit := &OpsAudit{Host: "test-go-" + dbtest.Hex(4)}
		t.Cleanup(func() { fx.Exec("DELETE FROM OpsAuditLog WHERE host = ?", audit.Host) })
		return fx, id, audit
	}
	state := func(fx dbtest.Fixture, id string) []string {
		return fx.Rows("SELECT u.bannedAt, (SELECT COUNT(*) FROM AuthSession s WHERE s.userId = u.id) AS sessions FROM `user` u WHERE u.id = ?", id)
	}

	t.Run("止めると、停止の印を付けてセッションを消し、前後の値を記録する", func(t *testing.T) {
		fx, id, audit := setup(t, 2)
		now := time.Date(2026, 10, 6, 1, 2, 3, 4_000_000, time.UTC)

		got, err := Suspend(ctx, fx.DB, id, now, audit)

		if err != nil {
			t.Fatal(err)
		}
		if got.BannedAtBefore != nil || got.BannedAt != "2026-10-06T01:02:03.004Z" || got.SessionsRemoved != 2 {
			t.Errorf("Suspend = %+v (BannedAtBefore=%v)", got, got.BannedAtBefore)
		}
		if want := `{"bannedAt":"2026-10-06 01:02:03.004","sessions":"0"}`; strings.Join(state(fx, id), "") != want {
			t.Errorf("DB = %v, want %s", state(fx, id), want)
		}
		records := fx.Rows("SELECT action, targetId, detail FROM OpsAuditLog WHERE host = ?", audit.Host)
		want := `{"action":"ban","detail":"{\"after\": {\"bannedAt\": \"2026-10-06T01:02:03.004Z\"}, \"before\": {\"bannedAt\": null}, \"sessionsRemoved\": 2}","targetId":"` + id + `"}`
		if len(records) != 1 || records[0] != want {
			t.Errorf("記録 = %v\nwant %s", records, want)
		}
	})

	t.Run("止まっている人を止め直しても、最初に止めた時刻は変えない", func(t *testing.T) {
		fx, id, _ := setup(t, 1)
		first := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		if _, err := Suspend(ctx, fx.DB, id, first, nil); err != nil {
			t.Fatal(err)
		}

		got, err := Suspend(ctx, fx.DB, id, first.Add(time.Hour), nil)

		if err != nil {
			t.Fatal(err)
		}
		if got.BannedAtBefore == nil || *got.BannedAtBefore != "2026-10-01T00:00:00.000Z" || got.BannedAt != "2026-10-01T00:00:00.000Z" {
			t.Errorf("Suspend = %+v (BannedAtBefore=%v)", got, got.BannedAtBefore)
		}
	})

	t.Run("記録を渡さなければ（管理画面）、記録は書かない", func(t *testing.T) {
		fx, id, _ := setup(t, 0)
		if _, err := Suspend(ctx, fx.DB, id, time.Now().UTC(), nil); err != nil {
			t.Fatal(err)
		}
		if err := Unsuspend(ctx, fx.DB, id, time.Now().UTC(), nil); err != nil {
			t.Fatal(err)
		}
		if got := fx.Rows("SELECT id FROM OpsAuditLog WHERE targetId = ?", id); len(got) != 0 {
			t.Errorf("記録 = %v", got)
		}
	})

	t.Run("解除すると停止の印を外し、記録する", func(t *testing.T) {
		fx, id, audit := setup(t, 0)
		fx.Exec("UPDATE `user` SET bannedAt = ? WHERE id = ?", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), id)

		if err := Unsuspend(ctx, fx.DB, id, time.Now().UTC(), audit); err != nil {
			t.Fatal(err)
		}

		if want := `{"bannedAt":null,"sessions":"0"}`; strings.Join(state(fx, id), "") != want {
			t.Errorf("DB = %v, want %s", state(fx, id), want)
		}
		if got := fx.Rows("SELECT action FROM OpsAuditLog WHERE host = ?", audit.Host); strings.Join(got, "") != `{"action":"unban"}` {
			t.Errorf("記録 = %v", got)
		}
	})

	t.Run("いない利用者は ErrNotFound で、何も記録しない", func(t *testing.T) {
		fx, _, audit := setup(t, 0)
		missing := "test-go-missing-" + dbtest.Hex(8)

		if _, err := Suspend(ctx, fx.DB, missing, time.Now().UTC(), audit); !errors.Is(err, ErrNotFound) {
			t.Errorf("Suspend err = %v", err)
		}
		if err := Unsuspend(ctx, fx.DB, missing, time.Now().UTC(), audit); !errors.Is(err, ErrNotFound) {
			t.Errorf("Unsuspend err = %v", err)
		}
		if got := fx.Rows("SELECT id FROM OpsAuditLog WHERE host = ?", audit.Host); len(got) != 0 {
			t.Errorf("記録 = %v", got)
		}
	})

	t.Run("記録が書けなければ、停止も解除も取り消す（操作と記録を1つのトランザクションで行う）", func(t *testing.T) {
		fx, id, _ := setup(t, 2)
		// host は VARCHAR(255)。長すぎる値で記録の INSERT だけを失敗させる。
		broken := &OpsAudit{Host: strings.Repeat("h", 256)}

		_, err := Suspend(ctx, fx.DB, id, time.Now().UTC(), broken)

		if err == nil || !strings.Contains(err.Error(), "止めていません") {
			t.Fatalf("err = %v", err)
		}
		if want := `{"bannedAt":null,"sessions":"2"}`; strings.Join(state(fx, id), "") != want {
			t.Errorf("記録が無いのに変わった: %v", state(fx, id))
		}

		fx.Exec("UPDATE `user` SET bannedAt = ? WHERE id = ?", time.Now().UTC(), id)
		if err := Unsuspend(ctx, fx.DB, id, time.Now().UTC(), broken); err == nil || !strings.Contains(err.Error(), "停止を戻していません") {
			t.Fatalf("err = %v", err)
		}
		if got := strings.Join(state(fx, id), ""); strings.Contains(got, `"bannedAt":null`) {
			t.Errorf("記録が無いのに停止が戻った: %s", got)
		}
	})
}
