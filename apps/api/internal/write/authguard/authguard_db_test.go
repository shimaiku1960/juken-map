//go:build dbtest

package authguard

import (
	"context"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// ログインの守りの操作を、本物の MySQL で確かめる。入口からの流れ（429・メールが届くか）は internal/feature/auth の auth_db_test.go にある。
func TestAuthGuardDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	t.Run("試行は上限まで通し、超えたら窓の残りを返す。窓が終われば数え直し、Clear で消える", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		rule := Rule{Name: "test:" + dbtest.Hex(4), Max: 3, Window: time.Minute}
		subject := " Someone@Example.test "
		t.Cleanup(func() { fx.Exec("DELETE FROM AuthThrottle WHERE bucket = ?", Bucket(rule, subject)) })
		for i := range rule.Max {
			if ok, _, err := Hit(ctx, db, rule, subject, now); err != nil || !ok {
				t.Fatalf("%d 回目: %v %v", i+1, ok, err)
			}
		}
		// 大文字・小文字と前後の空白は同じ単位に数える
		ok, retry, err := Hit(ctx, db, rule, "someone@example.test", now.Add(10*time.Second))
		if err != nil || ok || retry != 50*time.Second {
			t.Fatalf("上限を超えた試行: %v %v %v", ok, retry, err)
		}
		if ok, _, err := Hit(ctx, db, rule, subject, now.Add(rule.Window)); err != nil || !ok {
			t.Fatalf("窓が終わったのに止まる: %v %v", ok, err)
		}
		if err := Clear(ctx, db, rule, subject); err != nil {
			t.Fatal(err)
		}
		if got := fx.Rows("SELECT count FROM AuthThrottle WHERE bucket = ?", Bucket(rule, subject)); len(got) != 0 {
			t.Errorf("Clear のあとも残っている: %v", got)
		}
	})

	t.Run("同じ宛先へは1時間に上限まで。運営者への通知は宛先ごとの上限にかけない", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		recipient := "test-" + dbtest.Hex(16)
		t.Cleanup(func() { fx.Exec("DELETE FROM EmailSend WHERE recipientHash = ?", recipient) })
		for i := range EmailPerRecipientPerHour {
			if block, err := ReserveEmail(ctx, db, recipient, "verification", now); err != nil || block != EmailAllowed {
				t.Fatalf("%d 通目: %q %v", i+1, block, err)
			}
		}
		if block, err := ReserveEmail(ctx, db, recipient, "password-reset", now); err != nil || block != BlockedRecipient {
			t.Fatalf("上限を超えた: %q %v", block, err)
		}
		if block, err := ReserveEmail(ctx, db, recipient, KindAdminNewUser, now); err != nil || block != EmailAllowed {
			t.Fatalf("運営者への通知: %q %v", block, err)
		}
		if block, err := ReserveEmail(ctx, db, recipient, "verification", now.Add(time.Hour)); err != nil || block != EmailAllowed {
			t.Fatalf("1時間たっても止まる: %q %v", block, err)
		}
	})

	t.Run("state は1回だけ使える。プロバイダーが違う・期限切れなら使わない", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		hash := []byte(dbtest.Hex(16))
		t.Cleanup(func() { fx.Exec("DELETE FROM AuthOAuthState WHERE stateHash = ?", hash) })
		want := OAuthState{Verifier: "V", Nonce: "N", RedirectTo: "/dashboard"}
		if err := SaveOAuthState(ctx, db, hash, "google", want, now); err != nil {
			t.Fatal(err)
		}
		if got, err := ConsumeOAuthState(ctx, db, hash, "github", now); err != nil || got != nil {
			t.Fatalf("プロバイダーが違うのに使えた: %+v %v", got, err)
		}
		if got, err := ConsumeOAuthState(ctx, db, hash, "google", now.Add(OAuthStateTTL)); err != nil || got != nil {
			t.Fatalf("期限切れなのに使えた: %+v %v", got, err)
		}
		if got, err := ConsumeOAuthState(ctx, db, hash, "google", now); err != nil || got == nil || *got != want {
			t.Fatalf("使えない: %+v %v", got, err)
		}
		if got, err := ConsumeOAuthState(ctx, db, hash, "google", now); err != nil || got != nil {
			t.Fatalf("2回使えた: %+v %v", got, err)
		}
	})
}
