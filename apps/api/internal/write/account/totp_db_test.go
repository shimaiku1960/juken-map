//go:build dbtest

package account

import (
	"context"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 2段階認証の操作を、本物の MySQL で確かめる。入口からの流れ（設定・ログイン・予備コード）は
// internal/feature/auth の auth_db_test.go、運用のコマンド（reset-2fa）は incident_db_test.go にある。
func TestTOTPDB(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 1, 2, 3, 4_000_000, time.UTC)

	t.Run("設定し直すと予備コードが入れ替わり、有効にするのは1回だけ。同じステップは2回使えない", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		old, fresh := []byte(dbtest.Hex(16)), []byte(dbtest.Hex(16))
		if err := StartTOTPSetup(ctx, fx.DB, id, "SEALED-1", [][]byte{old}, now); err != nil {
			t.Fatal(err)
		}
		if err := StartTOTPSetup(ctx, fx.DB, id, "SEALED-2", [][]byte{fresh}, now); err != nil {
			t.Fatal(err)
		}
		for i, want := range []bool{true, false} {
			if ok, err := EnableTOTP(ctx, fx.DB, id, 100, now); err != nil || ok != want {
				t.Errorf("有効にする %d 回目 = %v %v, want %v", i+1, ok, err, want)
			}
		}
		for _, c := range []struct {
			step int64
			want bool
		}{{100, false}, {99, false}, {101, true}, {101, false}} {
			if ok, err := MarkTOTPStepUsed(ctx, fx.DB, id, c.step); err != nil || ok != c.want {
				t.Errorf("ステップ %d = %v %v, want %v", c.step, ok, err, c.want)
			}
		}
		for _, c := range []struct {
			name string
			hash []byte
			want bool
		}{{"古い予備コード", old, false}, {"新しい予備コード", fresh, true}, {"使った予備コード", fresh, false}} {
			if ok, err := UseBackupCode(ctx, fx.DB, id, c.hash); err != nil || ok != c.want {
				t.Errorf("%s = %v %v, want %v", c.name, ok, err, c.want)
			}
		}
		if got := fx.Rows("SELECT secret FROM AuthTotp WHERE userId = ?", id); len(got) != 1 || got[0] != `{"secret":"SEALED-2"}` {
			t.Errorf("秘密 = %v", got)
		}
	})

	t.Run("リセットすると TOTP・予備コード・途中の状態・セッションを消し、同じトランザクションで記録する", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: dbtest.Open(t)}
		id := fx.User()
		if err := StartTOTPSetup(ctx, fx.DB, id, "SEALED", [][]byte{[]byte(dbtest.Hex(16))}, now); err != nil {
			t.Fatal(err)
		}
		if err := CreateMFAChallenge(ctx, fx.DB, id, []byte(dbtest.Hex(16)), time.Minute, now); err != nil {
			t.Fatal(err)
		}
		fx.Exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
			dbtest.Hex(16), dbtest.Hex(16), id, now, now.Add(time.Hour), now)
		audit := &OpsAudit{Host: "test-go-" + dbtest.Hex(4)}
		t.Cleanup(func() { fx.Exec("DELETE FROM OpsAuditLog WHERE host = ?", audit.Host) })

		removed, err := ResetTwoFactor(ctx, fx.DB, id, now, audit)

		if err != nil || removed != 1 {
			t.Fatalf("ResetTwoFactor = %d, %v", removed, err)
		}
		got := fx.Rows("SELECT (SELECT COUNT(*) FROM AuthTotp WHERE userId = ?) AS totp,"+
			" (SELECT COUNT(*) FROM AuthBackupCode WHERE userId = ?) AS backup,"+
			" (SELECT COUNT(*) FROM AuthMfaChallenge WHERE userId = ?) AS challenges,"+
			" (SELECT COUNT(*) FROM AuthSession WHERE userId = ?) AS sessions,"+
			" (SELECT COUNT(*) FROM OpsAuditLog WHERE host = ? AND action = 'reset-2fa' AND targetId = ?) AS audit", id, id, id, id, audit.Host, id)
		if want := `{"audit":"1","backup":"0","challenges":"0","sessions":"0","totp":"0"}`; len(got) != 1 || got[0] != want {
			t.Errorf("リセットの後 = %v, want %s", got, want)
		}
	})
}
