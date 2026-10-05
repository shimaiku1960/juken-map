//go:build dbtest

package main

import (
	"context"
	"crypto/rand"
	"testing"
	"time"
)

// 期限は2000年にする。ほかのテストやほかの worktree が同じ DB に作る行（期限は今より後）を消さない。
var (
	expiredAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	cleanupAt = time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC)
	stillOkAt = time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC)
)

// expiringRows は6つの表に、期限 expiresAt の行を1つずつ作る。
func expiringRows(fx dbFixture, userID string, expiresAt time.Time) {
	fx.t.Helper()
	h := func() []byte {
		b := make([]byte, 32)
		rand.Read(b)
		return b
	}
	now := time.Now()
	fx.exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
		testHex(16), h(), userID, now, expiresAt, now)
	fx.exec("INSERT INTO AuthToken (tokenHash, purpose, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?, ?)",
		h(), tokenPurposePasswordReset, userID, now, expiresAt)
	fx.exec("INSERT INTO AuthMfaChallenge (tokenHash, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?)", h(), userID, now, expiresAt)
	state := h()
	fx.exec("INSERT INTO AuthOAuthState (stateHash, provider, codeVerifier, nonce, redirectTo, createdAt, expiresAt) VALUES (?, 'google', ?, ?, '/', ?, ?)",
		state, testHex(32), testHex(16), now, expiresAt)
	fx.t.Cleanup(func() { fx.exec("DELETE FROM AuthOAuthState WHERE stateHash = ?", state) })
	fx.exec("INSERT INTO LineLinkNonce (nonce, userId, expiresAt) VALUES (?, ?, ?)", testHex(16), userID, expiresAt)
	fx.exec("INSERT INTO LineOAuthAttempt (state, userId, nonce, codeVerifier, redirectUri, expiresAt) VALUES (?, ?, ?, ?, ?, ?)",
		testHex(16), userID, testHex(16), testHex(32), "https://juken-map.com/api/line/oauth/callback", expiresAt)
}

// remaining は表ごとに、期限 expiresAt の行の数を返す（AuthOAuthState は利用者を持たないので期限だけで数える）。
func remaining(t *testing.T, fx dbFixture, userID string, expiresAt time.Time) map[string]int {
	t.Helper()
	got := map[string]int{}
	for _, table := range expiredTables {
		if table.name == "AuthOAuthState" {
			got[table.name] = count(t, fx.db, "SELECT COUNT(*) FROM AuthOAuthState WHERE expiresAt = ?", expiresAt)
			continue
		}
		got[table.name] = count(t, fx.db, "SELECT COUNT(*) FROM `"+table.name+"` WHERE userId = ? AND expiresAt = ?", userID, expiresAt)
	}
	return got
}

func TestExpiredCleanupDB(t *testing.T) {
	db := openTestDB(t)
	fx := dbFixture{t: t, db: db}
	ctx := context.Background()

	t.Run("期限の切れた行だけを、上限ずつ繰り返して消す", func(t *testing.T) {
		userID := fx.user()
		// 期限切れを3組作り、上限2で消す（1回では消しきれない）。
		for range 3 {
			expiringRows(fx, userID, expiredAt)
		}
		expiringRows(fx, userID, stillOkAt)

		removed, err := deleteExpired(ctx, db, cleanupAt, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, table := range expiredTables {
			if removed[table.name] != 3 {
				t.Errorf("%s: 消した行 = %d, want 3", table.name, removed[table.name])
			}
		}
		for table, n := range remaining(t, fx, userID, expiredAt) {
			if n != 0 {
				t.Errorf("%s: 期限切れが %d 行残った", table, n)
			}
		}
		for table, n := range remaining(t, fx, userID, stillOkAt) {
			if n != 1 {
				t.Errorf("%s: 期限内の行 = %d, want 1", table, n)
			}
		}
	})

	t.Run("起動したときに1回消し、取り消されたら止まる", func(t *testing.T) {
		userID := fx.user()
		expiringRows(fx, userID, expiredAt)

		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			runExpiredCleanup(runCtx, db, func() time.Time { return cleanupAt })
			close(done)
		}()
		deadline := time.Now().Add(5 * time.Second)
		for {
			left := 0
			for _, n := range remaining(t, fx, userID, expiredAt) {
				left += n
			}
			if left == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("5秒たっても期限切れが %d 行残っている", left)
			}
			time.Sleep(50 * time.Millisecond)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("取り消しても止まらない")
		}
	})
}
