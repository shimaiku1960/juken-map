//go:build dbtest

// incident・grant-admin のコマンド（cli.go・incident.go）を本物の DB で確かめる（JUK-122）。
// 本番では手順書（docs/incident-response.md）からしか使わないので、いざというときに SQL や引数の読み方の誤りで
// 動かない、ということがないようにする。Node の incident-service.test.ts・user-service.test.ts から移した。
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"testing"
	"time"
)

type cliFixture struct {
	dbFixture
	st incidentStore
}

func newCLIFixture(t *testing.T) cliFixture {
	db := openTestDB(t)
	return cliFixture{dbFixture: dbFixture{t: t, db: db}, st: incidentStore{db: db, now: time.Now}}
}

// account は利用者を作り、セッションを sessions 件ぶら下げてメールアドレスと ID を返す。
func (fx cliFixture) account(sessions int, role string, verified bool) (email, id string) {
	fx.t.Helper()
	id = "test-go-incident-" + testHex(8)
	email = id + "@example.test"
	now := time.Now().UTC()
	fx.exec("INSERT INTO `user` (id, email, role, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)",
		id, email, role, verified, now, now)
	fx.t.Cleanup(func() { fx.db.Exec("DELETE FROM `user` WHERE id = ?", id) })
	for i := range sessions {
		token := make([]byte, 32)
		rand.Read(token)
		fx.exec(`INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt, ipAddress)
		         VALUES (?, ?, ?, ?, ?, ?, ?)`,
			testHex(16), token, id, now.Add(time.Duration(i)*time.Second), now.Add(24*time.Hour), now, fmt.Sprintf("192.0.2.%d", i))
	}
	return email, id
}

func (fx cliFixture) sessionCount(userID string) int {
	fx.t.Helper()
	var n int
	if err := fx.db.QueryRow("SELECT COUNT(*) FROM AuthSession WHERE userId = ?", userID).Scan(&n); err != nil {
		fx.t.Fatal(err)
	}
	return n
}

func (fx cliFixture) bannedAt(userID string) *string {
	fx.t.Helper()
	var v *string
	if err := fx.db.QueryRow("SELECT bannedAt FROM `user` WHERE id = ?", userID).Scan(&v); err != nil {
		fx.t.Fatal(err)
	}
	return v
}

// run はコマンドを実行し、終了コードと標準出力・標準エラーを返す。
func (fx cliFixture) run(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := dispatchCommand(context.Background(), fx.st, args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// mustRun は成功することを確かめて標準出力を返す。
func (fx cliFixture) mustRun(args ...string) string {
	fx.t.Helper()
	code, out, errOut := fx.run(args...)
	if code != 0 {
		fx.t.Fatalf("%v: 終了コード %d（%s）", args, code, errOut)
	}
	return out
}

func TestIncidentCommandDB(t *testing.T) {
	t.Run("sessions は、その人のセッションを作られた順に出す", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, _ := fx.account(2, "user", true)

		out := fx.mustRun("incident", "sessions", email)

		if !strings.Contains(out, "セッション 2 件") {
			t.Errorf("出力 = %q", out)
		}
		if first, second := strings.Index(out, "192.0.2.0"), strings.Index(out, "192.0.2.1"); first < 0 || second < first {
			t.Errorf("作られた順に並んでいない: %q", out)
		}
	})

	t.Run("revoke は、その人のセッションだけを消し、止めはしない", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(2, "user", true)
		_, other := fx.account(1, "user", true)

		out := fx.mustRun("incident", "revoke", email)

		if !strings.Contains(out, "2 件消しました") {
			t.Errorf("出力 = %q", out)
		}
		if fx.sessionCount(id) != 0 || fx.sessionCount(other) != 1 || fx.bannedAt(id) != nil {
			t.Error("その人のセッションだけを消していない、または止めた")
		}
	})

	t.Run("ban は、管理者でも止めてセッションを消し、止め直しても最初の日時を保つ", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(2, "admin", true)

		fx.mustRun("incident", "ban", email)
		first := fx.bannedAt(id)
		fx.st.now = func() time.Time { return time.Now().Add(time.Hour) }
		fx.mustRun("incident", "ban", email)

		if first == nil || fx.sessionCount(id) != 0 {
			t.Fatal("止めていない、またはセッションが残っている")
		}
		if again := fx.bannedAt(id); again == nil || *again != *first {
			t.Errorf("止め直して日時が変わった: %v → %v", *first, again)
		}
	})

	t.Run("unban は、止めたのを戻す", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(0, "user", true)
		fx.mustRun("incident", "ban", email)

		fx.mustRun("incident", "unban", email)

		if fx.bannedAt(id) != nil {
			t.Error("止めたままになっている")
		}
	})

	t.Run("revoke-admins は、管理者のセッションだけを全員分消す", func(t *testing.T) {
		fx := newCLIFixture(t)
		adminA, idA := fx.account(1, "admin", true)
		_, idB := fx.account(2, "admin", true)
		_, user := fx.account(1, "user", true)

		out := fx.mustRun("incident", "revoke-admins")

		if !strings.Contains(out, adminA) {
			t.Errorf("消した管理者に出ていない: %q", out)
		}
		if fx.sessionCount(idA) != 0 || fx.sessionCount(idB) != 0 || fx.sessionCount(user) != 1 {
			t.Error("管理者のセッションだけを消していない")
		}
	})

	t.Run("revoke-all は、全員のセッションを消す", func(t *testing.T) {
		fx := newCLIFixture(t)
		_, a := fx.account(1, "user", true)
		_, b := fx.account(2, "admin", true)

		fx.mustRun("incident", "revoke-all")

		if fx.sessionCount(a) != 0 || fx.sessionCount(b) != 0 {
			t.Error("セッションが残っている")
		}
	})

	t.Run("reset-2fa は、2段階認証を設定前に戻し、セッションを消す", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(1, "admin", true)
		now := time.Now().UTC()
		fx.exec("INSERT INTO AuthTotp (userId, secret, createdAt, enabledAt) VALUES (?, 'v0:secret', ?, ?)", id, now, now)
		code := make([]byte, 32)
		rand.Read(code)
		fx.exec("INSERT INTO AuthBackupCode (userId, codeHash) VALUES (?, ?)", id, code)

		fx.mustRun("incident", "reset-2fa", email)

		var left int
		fx.db.QueryRow("SELECT (SELECT COUNT(*) FROM AuthTotp WHERE userId = ?) + (SELECT COUNT(*) FROM AuthBackupCode WHERE userId = ?)", id, id).Scan(&left)
		if left != 0 || fx.sessionCount(id) != 0 {
			t.Errorf("残りの秘密=%d セッション=%d", left, fx.sessionCount(id))
		}
	})

	t.Run("いないメールアドレスなら何もせず、終了コード 1 で知らせる", func(t *testing.T) {
		fx := newCLIFixture(t)
		email := "missing-" + testHex(8) + "@example.test"
		for _, op := range []string{"sessions", "revoke", "ban", "unban", "reset-2fa"} {
			code, _, errOut := fx.run("incident", op, email)
			if code != 1 || !strings.Contains(errOut, "見つかりません") {
				t.Errorf("%s: 終了コード %d（%s）", op, code, errOut)
			}
		}
	})

	t.Run("操作やメールアドレスが無ければ使い方を出して終了コード 1", func(t *testing.T) {
		fx := newCLIFixture(t)
		for _, args := range [][]string{{"incident"}, {"incident", "ban"}, {"incident", "unknown", "a@example.test"}, {"grant-admin"}, {"unknown"}} {
			if code, _, errOut := fx.run(args...); code != 1 || errOut == "" {
				t.Errorf("%v: 終了コード %d（%s）", args, code, errOut)
			}
		}
	})
}

func TestGrantAdminCommandDB(t *testing.T) {
	t.Run("付け替えたら、その人のセッションをすべて消す（認証基準 10 の C4）", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(2, "user", true)
		_, other := fx.account(1, "user", true)

		out := fx.mustRun("grant-admin", email)

		var role string
		fx.db.QueryRow("SELECT role FROM `user` WHERE id = ?", id).Scan(&role)
		if role != "admin" || !strings.Contains(out, "user → admin") || !strings.Contains(out, "2 件消しました") {
			t.Errorf("role=%s 出力=%q", role, out)
		}
		if fx.sessionCount(id) != 0 || fx.sessionCount(other) != 1 {
			t.Error("その人のセッションだけを消していない")
		}
		if list := fx.mustRun("grant-admin", "--list"); !strings.Contains(list, email+"\t2段階認証: 未設定\tパスワード: なし") {
			t.Errorf("--list に出ていない: %q", list)
		}

		fx.mustRun("grant-admin", email, "--revoke")
		fx.db.QueryRow("SELECT role FROM `user` WHERE id = ?", id).Scan(&role)
		if role != "user" {
			t.Errorf("--revoke 後の role = %s", role)
		}
	})

	t.Run("メール確認前の人は管理者にせず、セッションも消さない", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(1, "user", false)

		code, _, errOut := fx.run("grant-admin", email)

		var role string
		fx.db.QueryRow("SELECT role FROM `user` WHERE id = ?", id).Scan(&role)
		if code != 1 || !strings.Contains(errOut, "メール確認が済んでいない") || role != "user" || fx.sessionCount(id) != 1 {
			t.Errorf("終了コード %d（%s） role=%s セッション=%d", code, errOut, role, fx.sessionCount(id))
		}
	})
}
