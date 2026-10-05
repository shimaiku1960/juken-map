//go:build dbtest

// incident・grant-admin のコマンド（cli.go・incident.go）を本物の DB で確かめる（JUK-122）。
// 本番では手順書（docs/incident-response.md）からしか使わないので、いざというときに SQL や引数の読み方の誤りで
// 動かない、ということがないようにする。Node の incident-service.test.ts・user-service.test.ts から移した。
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

type cliFixture struct {
	dbFixture
	st incidentStore
	// opsFrom は作った時点の OpsAuditLog の最大の id。これより後の行を、このテストが書いたものとして見て、終わったら消す。
	opsFrom int64
}

func newCLIFixture(t *testing.T) cliFixture {
	db := openTestDB(t)
	fx := cliFixture{dbFixture: dbFixture{t: t, db: db}, st: incidentStore{db: db, now: time.Now}}
	if err := db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM OpsAuditLog").Scan(&fx.opsFrom); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fx.exec("DELETE FROM OpsAuditLog WHERE id > ?", fx.opsFrom) })
	return fx
}

// opsRecord は OpsAuditLog の1行。
type opsRecord struct {
	Action    string
	TargetID  *string
	Detail    map[string]any
	RawDetail string
	Host      string
	CreatedAt time.Time
}

// opsRecords は、このテストのあいだに書かれた OpsAuditLog の行を、書かれた順に返す。
func (fx cliFixture) opsRecords() []opsRecord {
	fx.t.Helper()
	rows, err := fx.db.Query("SELECT action, targetId, detail, host, createdAt FROM OpsAuditLog WHERE id > ? ORDER BY id", fx.opsFrom)
	if err != nil {
		fx.t.Fatal(err)
	}
	defer rows.Close()
	var records []opsRecord
	for rows.Next() {
		var (
			r         opsRecord
			createdAt string
		)
		if err := rows.Scan(&r.Action, &r.TargetID, &r.RawDetail, &r.Host, &createdAt); err != nil {
			fx.t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(r.RawDetail), &r.Detail); err != nil {
			fx.t.Fatal(err)
		}
		if r.CreatedAt, err = time.Parse(time.RFC3339Nano, isoFromDatetime(createdAt)); err != nil {
			fx.t.Fatal(err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		fx.t.Fatal(err)
	}
	return records
}

// wantOneOps は、このテストのあいだに書かれた記録が1行だけで、その操作と対象（全員が対象なら ""）が合うことを確かめる。
func (fx cliFixture) wantOneOps(action, targetID string) opsRecord {
	fx.t.Helper()
	records := fx.opsRecords()
	if len(records) != 1 {
		fx.t.Fatalf("OpsAuditLog の行 = %d 件（%+v）", len(records), records)
	}
	r := records[0]
	gotTarget := ""
	if r.TargetID != nil {
		gotTarget = *r.TargetID
	}
	if r.Action != action || gotTarget != targetID || r.Host == "" {
		fx.t.Errorf("記録 = action %s target %q host %q、want action %s target %q", r.Action, gotTarget, r.Host, action, targetID)
	}
	return r
}

// account は利用者を作り、セッションを sessions 件ぶら下げてメールアドレスと ID を返す。
func (fx cliFixture) account(sessions int, role string, verified bool) (email, id string) {
	fx.t.Helper()
	id = "test-go-incident-" + testHex(8)
	email = id + "@example.test"
	now := time.Now().UTC()
	fx.exec("INSERT INTO `user` (id, email, role, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)",
		id, email, role, verified, now, now)
	fx.t.Cleanup(func() { fx.exec("DELETE FROM `user` WHERE id = ?", id) })
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
		if r := fx.wantOneOps("revoke", id); r.Detail["sessionsRemoved"] != 2.0 {
			t.Errorf("detail = %s", r.RawDetail)
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

	t.Run("変えた操作は、何を・誰に・前後の値・いつを OpsAuditLog に残し、見るだけの操作は残さない（セキュリティ基準 H4）", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(2, "user", true)
		start := time.Now().UTC().Truncate(time.Millisecond)

		fx.mustRun("incident", "sessions", email)
		fx.mustRun("incident", "ban", email)
		banned := fx.bannedAt(id)
		if banned == nil {
			t.Fatal("止めていない")
		}
		bannedAt := isoFromDatetime(*banned)
		fx.mustRun("incident", "unban", email)

		records := fx.opsRecords()
		if len(records) != 2 || records[0].Action != "ban" || records[1].Action != "unban" {
			t.Fatalf("記録 = %+v", records)
		}
		ban, unban := records[0], records[1]
		if ban.TargetID == nil || *ban.TargetID != id || ban.Host == "" {
			t.Errorf("誰に・どこで が分からない: target %v host %q", ban.TargetID, ban.Host)
		}
		if ban.CreatedAt.Before(start) || ban.CreatedAt.After(time.Now().Add(time.Second)) {
			t.Errorf("いつ = %v（開始 %v）", ban.CreatedAt, start)
		}
		wantBan := fmt.Sprintf(`{"after": {"bannedAt": %q}, "before": {"bannedAt": null}, "sessionsRemoved": 2}`, bannedAt)
		wantUnban := fmt.Sprintf(`{"after": {"bannedAt": null}, "before": {"bannedAt": %q}}`, bannedAt)
		if ban.RawDetail != wantBan || unban.RawDetail != wantUnban {
			t.Errorf("前後の値:\n ban   %s\n unban %s", ban.RawDetail, unban.RawDetail)
		}
		if strings.Contains(ban.RawDetail+unban.RawDetail, email) {
			t.Error("記録にメールアドレスが入っている")
		}

		// incident log で、新しい順に「いつ・何を・誰に・どこで・前後の値」を引ける。log 自体は見るだけなので残さない。
		lines := strings.Split(fx.mustRun("incident", "log"), "\n")
		wantFirst := strings.Join([]string{isoMillis(unban.CreatedAt), "unban", id, unban.Host, wantUnban}, "\t")
		wantSecond := strings.Join([]string{isoMillis(ban.CreatedAt), "ban", id, ban.Host, wantBan}, "\t")
		if len(lines) < 2 || lines[0] != wantFirst || lines[1] != wantSecond {
			t.Errorf("incident log の先頭2行:\n got  %q\n want %q\n      %q", lines[:min(2, len(lines))], wantFirst, wantSecond)
		}
		if n := len(fx.opsRecords()); n != 2 {
			t.Errorf("log のあとの記録 = %d 件", n)
		}
	})

	t.Run("記録を書くとき、1年を過ぎた行を消し、1年以内の行は残す", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(0, "user", true)
		now := time.Now().UTC()
		insert := func(age time.Duration) {
			fx.exec("INSERT INTO OpsAuditLog (action, targetId, detail, host, createdAt) VALUES ('revoke', ?, '{}', 'test', ?)", id, now.Add(-age))
		}
		insert(opsAuditRetention + time.Hour)
		insert(opsAuditRetention - time.Hour)

		fx.mustRun("incident", "revoke", email)

		records := fx.opsRecords()
		if len(records) != 2 || records[0].Host != "test" || records[1].Host == "test" {
			t.Errorf("残った行 = %+v", records)
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
		r := fx.wantOneOps("revoke-admins", "")
		if !strings.Contains(r.RawDetail, idA) || !strings.Contains(r.RawDetail, idB) || strings.Contains(r.RawDetail, user) || strings.Contains(r.RawDetail, adminA) {
			t.Errorf("対象の管理者の userId だけが並んでいない: %s", r.RawDetail)
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
		fx.wantOneOps("revoke-all", "")
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
		if err := fx.db.QueryRow("SELECT (SELECT COUNT(*) FROM AuthTotp WHERE userId = ?) + (SELECT COUNT(*) FROM AuthBackupCode WHERE userId = ?)", id, id).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left != 0 || fx.sessionCount(id) != 0 {
			t.Errorf("残りの秘密=%d セッション=%d", left, fx.sessionCount(id))
		}
		fx.wantOneOps("reset-2fa", id)
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
		if records := fx.opsRecords(); len(records) != 0 {
			t.Errorf("何も変えていないのに記録が残った: %+v", records)
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
		if err := fx.db.QueryRow("SELECT role FROM `user` WHERE id = ?", id).Scan(&role); err != nil {
			t.Fatal(err)
		}
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
		if err := fx.db.QueryRow("SELECT role FROM `user` WHERE id = ?", id).Scan(&role); err != nil {
			t.Fatal(err)
		}
		if role != "user" {
			t.Errorf("--revoke 後の role = %s", role)
		}

		// 権限の変更は前後の role を残す。--list は見るだけなので残さない。
		records := fx.opsRecords()
		if len(records) != 2 {
			t.Fatalf("記録 = %+v", records)
		}
		for i, want := range []string{
			`{"after": {"role": "admin"}, "before": {"role": "user"}, "sessionsRemoved": 2}`,
			`{"after": {"role": "user"}, "before": {"role": "admin"}, "sessionsRemoved": 0}`,
		} {
			if r := records[i]; r.Action != "set-role" || r.TargetID == nil || *r.TargetID != id || r.RawDetail != want {
				t.Errorf("%d 件目 = %s %v %s", i+1, r.Action, r.TargetID, r.RawDetail)
			}
		}
	})

	t.Run("メール確認前の人は管理者にせず、セッションも消さない", func(t *testing.T) {
		fx := newCLIFixture(t)
		email, id := fx.account(1, "user", false)

		code, _, errOut := fx.run("grant-admin", email)

		var role string
		if err := fx.db.QueryRow("SELECT role FROM `user` WHERE id = ?", id).Scan(&role); err != nil {
			t.Fatal(err)
		}
		if code != 1 || !strings.Contains(errOut, "メール確認が済んでいない") || role != "user" || fx.sessionCount(id) != 1 {
			t.Errorf("終了コード %d（%s） role=%s セッション=%d", code, errOut, role, fx.sessionCount(id))
		}
		if records := fx.opsRecords(); len(records) != 0 {
			t.Errorf("断ったのに記録が残った: %+v", records)
		}
	})
}
