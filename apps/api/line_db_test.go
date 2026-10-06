//go:build dbtest

// LINE 連携の SQL（sqlLineStore）を本物の DB に流して確かめる。ふだんの go test では動かない
// （dbtest タグ。`pnpm test:go-db` と CI の check のジョブが動かす）。
// 利用者はテストごとに作り、最後に消す（LINE の表は user の削除で一緒に消える）。
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

func TestLineStoreDB(t *testing.T) {
	db := dbtest.Open(t)
	st := &sqlLineStore{db: db}
	ctx := context.Background()

	newUser := func(t *testing.T) string {
		t.Helper()
		id := "test-line-" + randomHex(8)
		now := time.Now()
		if _, err := db.Exec("INSERT INTO `user` (id, email, createdAt, updatedAt) VALUES (?, ?, ?, ?)",
			id, id+"@example.test", now, now); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { newDBFixture(t, db).Exec("DELETE FROM `user` WHERE id = ?", id) })
		return id
	}
	lineUser := func() string { return "U" + randomHex(16) }

	t.Run("nonce で連携し、使い捨てにする。期限切れと別の人の LINE は断る", func(t *testing.T) {
		alice, bob, lineID := newUser(t), newUser(t), lineUser()
		if err := st.issueLinkNonce(ctx, alice, "n-"+alice); err != nil {
			t.Fatal(err)
		}
		mustResult(t, st, "n-"+alice, lineID, accountLinkLinked)
		mustResult(t, st, "n-"+alice, lineID, accountLinkExpired) // 2回目は使えない
		if ok, _ := st.isConnected(ctx, alice); !ok {
			t.Fatal("連携されていない")
		}
		if ok, _ := st.isLineUserConnected(ctx, lineID); !ok {
			t.Fatal("LINE 側から引けない")
		}

		// 同じ LINE を bob が連携しようとしても断る（nonce は消す）
		if err := st.issueLinkNonce(ctx, bob, "n-"+bob); err != nil {
			t.Fatal(err)
		}
		mustResult(t, st, "n-"+bob, lineID, accountLinkTaken)
		mustResult(t, st, "n-"+bob, lineID, accountLinkExpired)

		// 期限切れ
		newDBFixture(t, db).Exec("INSERT INTO LineLinkNonce (nonce, userId, expiresAt) VALUES (?, ?, ?)",
			"old-"+bob, bob, time.Now().Add(-time.Second))
		mustResult(t, st, "old-"+bob, lineUser(), accountLinkExpired)
	})

	t.Run("連携済みなら新しい LINE へ付け替える", func(t *testing.T) {
		alice, first, second := newUser(t), lineUser(), lineUser()
		if ok, err := st.linkVerifiedLineUser(ctx, alice, first); err != nil || !ok {
			t.Fatal(ok, err)
		}
		if ok, err := st.linkVerifiedLineUser(ctx, alice, second); err != nil || !ok {
			t.Fatal(ok, err)
		}
		if ok, _ := st.isLineUserConnected(ctx, first); ok {
			t.Error("古い LINE が残っている")
		}
		// 同じ LINE へもう一度（値が同じでも linkedAt が変わるので UPDATE で済む。INSERT で重複にならない）
		if ok, err := st.linkVerifiedLineUser(ctx, alice, second); err != nil || !ok {
			t.Fatal(ok, err)
		}
		bob := newUser(t)
		if ok, err := st.linkVerifiedLineUser(ctx, bob, second); err != nil || ok {
			t.Fatalf("別の人の LINE で連携できた: %v %v", ok, err)
		}
	})

	t.Run("同じ nonce が同時に届いても、連携は1回だけ", func(t *testing.T) {
		alice, lineID := newUser(t), lineUser()
		if err := st.issueLinkNonce(ctx, alice, "n-"+alice); err != nil {
			t.Fatal(err)
		}
		results := make([]accountLinkResult, 5)
		var wg sync.WaitGroup
		for i := range results {
			wg.Go(func() {
				r, err := st.completeAccountLink(ctx, "n-"+alice, lineID)
				if err != nil {
					t.Error(err)
				}
				results[i] = r
			})
		}
		wg.Wait()
		linked := 0
		for _, r := range results {
			if r == accountLinkLinked {
				linked++
			}
		}
		if linked != 1 {
			t.Fatalf("results = %v", results)
		}
	})

	t.Run("OAuth の試行は1人1つ。期限切れは DB の時計で判定する", func(t *testing.T) {
		alice := newUser(t)
		a := oauthAttempt{UserID: alice, Nonce: "N", CodeVerifier: "V", RedirectURI: "https://juken-map.com/api/line/oauth/callback"}
		if err := st.startOAuthAttempt(ctx, "s1-"+alice, a); err != nil {
			t.Fatal(err)
		}
		if err := st.startOAuthAttempt(ctx, "s2-"+alice, a); err != nil {
			t.Fatal(err)
		}
		if got, _ := st.findOAuthAttempt(ctx, "s1-"+alice); got != nil {
			t.Error("古い試行が残っている")
		}
		got, err := st.findOAuthAttempt(ctx, "s2-"+alice)
		if err != nil || got == nil || got.Expired || got.CodeVerifier != "V" || got.RedirectURI != a.RedirectURI {
			t.Fatalf("attempt = %+v, err = %v", got, err)
		}
		newDBFixture(t, db).Exec("UPDATE LineOAuthAttempt SET expiresAt = ? WHERE state = ?", time.Now().Add(-time.Second), "s2-"+alice)
		if got, _ := st.findOAuthAttempt(ctx, "s2-"+alice); got == nil || !got.Expired {
			t.Fatalf("期限切れにならない: %+v", got)
		}
		if err := st.discardOAuthAttempt(ctx, "s2-"+alice); err != nil {
			t.Fatal(err)
		}
		if got, _ := st.findOAuthAttempt(ctx, "s2-"+alice); got != nil {
			t.Error("捨てた試行が残っている")
		}
	})

	t.Run("解除で LINE 通知も落とし、nonce と試行も消す", func(t *testing.T) {
		alice := newUser(t)
		now := time.Now()
		newDBFixture(t, db).Exec(`INSERT INTO NotificationPreference (userId, morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled, updatedAt)
			VALUES (?, TRUE, TRUE, TRUE, TRUE, ?)`, alice, now)
		if _, err := st.linkVerifiedLineUser(ctx, alice, lineUser()); err != nil {
			t.Fatal(err)
		}
		if err := st.issueLinkNonce(ctx, alice, "n-"+alice); err != nil {
			t.Fatal(err)
		}
		if err := st.startOAuthAttempt(ctx, "s-"+alice, oauthAttempt{UserID: alice, Nonce: "N", CodeVerifier: "V", RedirectURI: "r"}); err != nil {
			t.Fatal(err)
		}

		if err := st.disconnect(ctx, alice); err != nil {
			t.Fatal(err)
		}
		var email, lineMorning, lineEvening bool
		if err := db.QueryRow("SELECT morningEnabled, lineMorningEnabled, lineEveningEnabled FROM NotificationPreference WHERE userId = ?", alice).
			Scan(&email, &lineMorning, &lineEvening); err != nil {
			t.Fatal(err)
		}
		if !email || lineMorning || lineEvening {
			t.Errorf("email=%v lineMorning=%v lineEvening=%v（メールは残し、LINE だけ落とす）", email, lineMorning, lineEvening)
		}
		for _, table := range []string{"LineConnection", "LineLinkNonce", "LineOAuthAttempt"} {
			if n := count(t, db, "SELECT COUNT(*) FROM "+table+" WHERE userId = ?", alice); n != 0 {
				t.Errorf("%s に %d 行残っている", table, n)
			}
		}
	})

	t.Run("Webhook のイベントは1回だけ印が入る。消せばまた入る", func(t *testing.T) {
		id := "test-" + randomHex(8)
		t.Cleanup(func() { newDBFixture(t, db).Exec("DELETE FROM LineWebhookEvent WHERE webhookEventId = ?", id) })
		if dup, err := st.markWebhookEvent(ctx, id); err != nil || dup {
			t.Fatal(dup, err)
		}
		if dup, err := st.markWebhookEvent(ctx, id); err != nil || !dup {
			t.Fatalf("2回目が重複にならない: %v %v", dup, err)
		}
		if err := st.unmarkWebhookEvent(ctx, id); err != nil {
			t.Fatal(err)
		}
		if dup, err := st.markWebhookEvent(ctx, id); err != nil || dup {
			t.Fatal(dup, err)
		}

		// 保持期間を過ぎた印は、次の Webhook のときに消える
		old := "test-old-" + randomHex(8)
		newDBFixture(t, db).Exec("INSERT INTO LineWebhookEvent (webhookEventId, createdAt) VALUES (?, ?)", old, time.Now().Add(-lineWebhookEventRetention-time.Hour))
		if _, err := st.markWebhookEvent(ctx, "test-"+randomHex(8)); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "SELECT COUNT(*) FROM LineWebhookEvent WHERE webhookEventId = ?", old); n != 0 {
			t.Error("古い印が消えていない")
		}
		newDBFixture(t, db).Exec("DELETE FROM LineWebhookEvent WHERE webhookEventId LIKE 'test-%'")
	})
}

func mustResult(t *testing.T, st *sqlLineStore, nonce, lineUserID string, want accountLinkResult) {
	t.Helper()
	got, err := st.completeAccountLink(context.Background(), nonce, lineUserID)
	if err != nil || got != want {
		t.Fatalf("completeAccountLink(%s) = %v, %v; want %v", nonce, got, err, want)
	}
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
