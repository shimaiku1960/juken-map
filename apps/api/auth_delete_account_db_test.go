//go:build dbtest

// 退会（06 G3、JUK-123）を本物の MySQL に流すテスト。
package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
)

func TestAuthDBDeleteAccount(t *testing.T) {
	e := newAuthEnv(t)

	t.Run("パスワードを入れ直すと退会でき、Cookie が消えて本人へ知らせる", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		other := e.signedIn(email, authTestPassword)

		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"password": "not my passphrase!!"}), 400, "INVALID_PASSWORD")
		if e.fx.count("SELECT COUNT(*) FROM `user` WHERE id = ?", id) != 1 {
			t.Fatal("パスワードが違うのに消えた")
		}

		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"password": authTestPassword}), 200, "")
		if e.fx.count("SELECT COUNT(*) FROM `user` WHERE id = ?", id) != 0 {
			t.Fatal("利用者が残っている")
		}
		if _, ok := b.cookies[sessionCookieName]; ok {
			t.Fatal("セッションの Cookie が消えていない")
		}
		if other.sessionEmail() != "" {
			t.Fatal("ほかの端末のセッションが残っている")
		}
		e.mails.last(t, email, "退会の手続きが完了しました")
		expectStatus(t, e.browser().do("POST", "/api/auth/sign-in", map[string]string{"email": email, "password": authTestPassword}), 401, "")
	})

	t.Run("2段階認証を有効にしていれば、コードも要る", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		_, backup := e.enableMFA(b)

		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"password": authTestPassword}), 400, "MFA_CODE_REQUIRED")
		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"password": authTestPassword, "code": "000000"}), 400, "INVALID_CODE")
		if e.fx.count("SELECT COUNT(*) FROM `user` WHERE id = ?", id) != 1 {
			t.Fatal("コードが無い・違うのに消えた")
		}
		// 予備コードでも通る（認証アプリを失くした人も退会できる）。
		expectStatus(t, b.do("POST", "/api/auth/delete-account",
			map[string]string{"password": authTestPassword, "code": backup[0], "method": "backup"}), 200, "")
		if e.fx.count("SELECT COUNT(*) FROM `user` WHERE id = ?", id) != 0 {
			t.Fatal("利用者が残っている")
		}
	})

	t.Run("2段階認証は認証アプリのコードでも通る", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		secret, _ := e.enableMFA(b)
		// 有効にしたときのコードと同じステップは使用済みなので、次のステップへ進める。
		e.clock.Advance(totpPeriod)
		expectStatus(t, b.do("POST", "/api/auth/delete-account",
			map[string]string{"password": authTestPassword, "code": totpCode(secret, totpStep(e.clock.Now()))}), 200, "")
		if e.fx.count("SELECT COUNT(*) FROM `user` WHERE id = ?", id) != 0 {
			t.Fatal("利用者が残っている")
		}
	})

	t.Run("パスワードの無い人はメールアドレスを打ち込む", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		e.fx.Exec("DELETE FROM AuthPassword WHERE userId = ?", id)

		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"email": "someone@else.example"}), 400, "EMAIL_MISMATCH")
		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"email": ""}), 400, "EMAIL_MISMATCH")
		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"email": "  " + strings.ToUpper(email) + " "}), 200, "")
		if e.fx.count("SELECT COUNT(*) FROM `user` WHERE id = ?", id) != 0 {
			t.Fatal("利用者が残っている")
		}
	})

	t.Run("管理者は退会できない", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		e.fx.Exec("UPDATE `user` SET role = 'admin' WHERE id = ?", id)
		b := e.signedIn(email, authTestPassword)
		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"password": authTestPassword}), 409, "ADMIN_CANNOT_DELETE")
		if e.fx.count("SELECT COUNT(*) FROM `user` WHERE id = ?", id) != 1 {
			t.Fatal("管理者が消えた")
		}
	})

	t.Run("ログインしていなければ 401", func(t *testing.T) {
		expectStatus(t, e.browser().do("POST", "/api/auth/delete-account", map[string]string{"password": authTestPassword}), 401, "")
	})
}

// TestAuthDBDeleteLeavesNoRows は、退会（本人）と削除（管理者）のどちらでも、利用者を指す表すべてに
// 行を作ってから消し、どの表のどの文字列の列にも本人の ID とメールアドレスが残らないことを確かめる。
// 利用者を指す表が増えたのに fillEveryUserTable に行を足していなければ落ちる（新しい表が消えるかを確かめるため）。
func TestAuthDBDeleteLeavesNoRows(t *testing.T) {
	e := newAuthEnv(t)
	facultyID := e.fx.University()

	t.Run("本人の退会", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		b := e.signedIn(email, authTestPassword)
		fillEveryUserTable(t, e.fx, id, email, facultyID)
		expectStatus(t, b.do("POST", "/api/auth/delete-account", map[string]string{"password": authTestPassword}), 200, "")
		expectNoTrace(t, e.fx, id, email)
	})

	t.Run("管理者の削除", func(t *testing.T) {
		email := e.newEmail()
		id := e.signUpVerified(email, authTestPassword)
		e.signedIn(email, authTestPassword)
		fillEveryUserTable(t, e.fx, id, email, facultyID)
		// 管理画面の削除（DELETE /api/admin/users/{id}）は account.DeleteUser をそのまま呼ぶ。
		if _, err := account.DeleteUser(context.Background(), e.db, id); err != nil {
			t.Fatal(err)
		}
		expectNoTrace(t, e.fx, id, email)
	})
}

// fillEveryUserTable は、利用者を指す表すべてに id の行を作る。作ったあと、外部キーで user を指す表に
// 1行も無いものがあれば落とす。
func fillEveryUserTable(t *testing.T, fx dbFixture, id, email string, facultyID int64) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	later := now.Add(time.Hour)
	h := func() []byte { return randomBytes(32) }
	fx.Textbook(id)
	fx.StudyPlan(id)
	fx.StudyLog(id)
	fx.FinalGoal(id, facultyID)
	fx.LineConnection(id)
	fx.Exec("INSERT INTO LineLinkNonce (nonce, userId, expiresAt) VALUES (?, ?, ?)", dbtest.Hex(16), id, later)
	fx.Exec("INSERT INTO LineOAuthAttempt (state, userId, nonce, codeVerifier, redirectUri, expiresAt) VALUES (?, ?, ?, ?, ?, ?)",
		dbtest.Hex(16), id, dbtest.Hex(16), dbtest.Hex(32), "https://juken-map.com/api/line/oauth/callback", later)
	fx.Exec("INSERT INTO NotificationPreference (userId, updatedAt) VALUES (?, ?)", id, now)
	fx.Exec("INSERT INTO NotificationDelivery (userId, date, slot) VALUES (?, ?, ?)", id, now, "morning")
	fx.Exec("INSERT INTO AuthIdentity (provider, providerUserId, userId, createdAt) VALUES ('google', ?, ?, ?)", "g-"+dbtest.Hex(6), id, now)
	fx.Exec("INSERT INTO AuthToken (tokenHash, purpose, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?, ?)", h(), tokenPurposePasswordReset, id, now, later)
	fx.Exec("INSERT INTO AuthMfaChallenge (tokenHash, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?)", h(), id, now, later)
	// 有効にしていない（enabledAt が NULL）ので、退会でコードは求められない。
	fx.Exec("INSERT INTO AuthTotp (userId, secret, createdAt) VALUES (?, ?, ?)", id, "v0:"+dbtest.Hex(16), now)
	fx.Exec("INSERT INTO AuthBackupCode (userId, codeHash) VALUES (?, ?)", id, h())

	for _, ref := range userReferences(t, fx) {
		if fx.count("SELECT COUNT(*) FROM `"+ref.table+"` WHERE `"+ref.column+"` = ?", id) == 0 {
			t.Errorf("%s.%s が user を指しているのに、テストの行が無い。fillEveryUserTable に足して、退会で消えることを確かめる", ref.table, ref.column)
		}
	}
}

type columnRef struct{ table, column string }

// userReferences は外部キーで user を指す列の一覧。
func userReferences(t *testing.T, fx dbFixture) []columnRef {
	t.Helper()
	rows, err := fx.DB.Query(`SELECT TABLE_NAME, COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE() AND REFERENCED_TABLE_NAME = 'user' ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []columnRef
	for rows.Next() {
		var r columnRef
		if err := rows.Scan(&r.table, &r.column); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		t.Fatal("user を指す外部キーが1つも見つからない（information_schema を読めていない）")
	}
	return out
}

// expectNoTrace は、すべての表の文字列の列に id とメールアドレスが含まれていないことを確かめる。
// 外部キーの無い表（回数制限・メールの記録など）も見る。そうした表は宛先や利用者を SHA-256 にして持つので当たらない。
func expectNoTrace(t *testing.T, fx dbFixture, id, email string) {
	t.Helper()
	rows, err := fx.DB.Query(`SELECT TABLE_NAME, COLUMN_NAME FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND DATA_TYPE IN ('char', 'varchar', 'text', 'mediumtext', 'longtext') ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	var cols []columnRef
	for rows.Next() {
		var c columnRef
		if err := rows.Scan(&c.table, &c.column); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	rows.Close()
	if len(cols) < 20 {
		t.Fatalf("文字列の列が %d 本しか見つからない（information_schema を読めていない）", len(cols))
	}
	for _, c := range cols {
		q := "SELECT COUNT(*) FROM `" + c.table + "` WHERE INSTR(`" + c.column + "`, ?) > 0 OR INSTR(`" + c.column + "`, ?) > 0"
		if n := fx.count(q, id, email); n != 0 {
			t.Errorf("%s.%s に本人の ID かメールアドレスが %d 行残っている", c.table, c.column, n)
		}
	}
}
