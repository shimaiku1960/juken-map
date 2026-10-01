//go:build parity

// 利用者の管理（JUK-78）を本物の DB に流し、Node と Go の応答を比べる。parity.sh から動かす。
//
// 管理 API は2段階認証を通した管理者のセッションが要るので、テストの中で管理者と、その session の行
// （twoFactorVerified = TRUE）を作り、BETTER_AUTH_SECRET で署名した Cookie で呼ぶ。
// 停止・削除は1回しかできないので、同じ形の相手を Node 用と Go 用に1人ずつ作り、それぞれに送った応答を
// 比べる（相手ごとに違う id・email・bannedAt は、有るか無いかだけを見る）。作った行は最後に消す。
package main

import (
	"context"
	"database/sql"
	stdjson "encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

type adminFixture struct {
	t          *testing.T
	db         *sql.DB
	secret     string
	cookieName string
}

// user は利用者を1人作る。消すのはテストの最後（ぶら下がる行は CASCADE で消える）。
func (f *adminFixture) user(role string) (id, email string) {
	f.t.Helper()
	id = "test-admin-" + randomHex(8)
	email = id + "@example.test"
	now := time.Now()
	if _, err := f.db.Exec("INSERT INTO `user` (id, email, role, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, TRUE, ?, ?)",
		id, email, role, now, now); err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { f.db.Exec("DELETE FROM `user` WHERE id = ?", id) })
	return id, email
}

// session はその利用者のセッションを作り、Cookie を返す。
func (f *adminFixture) session(userID string, twoFactor bool) string {
	f.t.Helper()
	token := randomHex(16)
	now := time.Now()
	if _, err := f.db.Exec(
		"INSERT INTO session (id, userId, expiresAt, token, createdAt, updatedAt, twoFactorVerified) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test-"+token, userID, now.Add(time.Hour), token, now, now, twoFactor); err != nil {
		f.t.Fatal(err)
	}
	return f.cookieName + "=" + sign(token, f.secret)
}

// studyLogs はその利用者の学習記録を n 件作る（削除で一緒に消える数を比べるため）。
func (f *adminFixture) studyLogs(userID string, n int) {
	f.t.Helper()
	now := time.Now()
	for range n {
		if _, err := f.db.Exec("INSERT INTO StudyLog (userId, date, minutes, createdAt, updatedAt) VALUES (?, ?, 30, ?, ?)",
			userID, now.Truncate(24*time.Hour), now, now); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *adminFixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(query, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func newAdminFixture(t *testing.T) (*adminFixture, *sql.DB) {
	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cookieName, _, _ := strings.Cut(strings.Fields(os.Getenv("PARITY_COOKIES"))[0], "=")
	return &adminFixture{t: t, db: db, secret: os.Getenv("BETTER_AUTH_SECRET"), cookieName: cookieName}, db
}

// compareAdmin は Node と Go に送り、masked の項目は有るか無いかだけを比べる。
func compareAdmin(t *testing.T, env parityEnvironment, node, gon parityRequest, masked ...string) response {
	t.Helper()
	a, b := send(t, env.node, node), send(t, env.goURL, gon)
	for _, key := range masked {
		maskKey(a.body, key)
		maskKey(b.body, key)
	}
	if diffs := compareResponses(a, b); len(diffs) > 0 {
		t.Fatalf("食い違い（Node → Go）:\n  %s", strings.Join(diffs, "\n  "))
	}
	return b
}

func TestParityAdminUsers(t *testing.T) {
	env := parityEnv(t)
	fx, _ := newAdminFixture(t)
	adminID, _ := fx.user("admin")
	admin := fx.session(adminID, true)
	otherAdminID, _ := fx.user("admin")
	json := map[string]string{"Content-Type": "application/json"}

	same := func(name, method, path, cookie, body string, header map[string]string) {
		t.Run(name, func(t *testing.T) {
			pr := parityRequest{method: method, path: path, cookie: cookie, body: body, header: header}
			compareAdmin(t, env, pr, pr)
		})
	}

	// 管理者以外は、どのルートもハンドラまで来ない（未ログイン 401・一般 403・2段階認証なし 403）
	noTwoFactor := fx.session(adminID, false)
	routes := [][2]string{
		{"GET", "/api/admin/overview"}, {"GET", "/api/admin/users"}, {"POST", "/api/admin/users/x/ban"},
		{"POST", "/api/admin/users/x/unban"}, {"DELETE", "/api/admin/users/x"},
	}
	for _, r := range routes {
		for _, c := range []struct {
			who    string
			cookie string
		}{{"未ログイン", ""}, {"一般の利用者", env.cookiesFor(firstUser)[0]}, {"2段階認証なし", noTwoFactor}} {
			same(fmt.Sprintf("%s %s（%s）", r[0], r[1], c.who), r[0], r[1], c.cookie, `{}`, json)
		}
	}

	// 読み取り。一覧はこのテストで作った利用者も含めて、同じ DB を同じ規則で数える
	// 概要は、学習記録が多い手元の DB（合成データで 1,000万件超）だと集計に10秒以上かかり、Go は1リクエストの
	// 上限（requestTimeout）で 500 にする（Node は待ち続けて返す）。SQL が同じ結果を返すかは、上限を通さずに
	// Go の SQL を直接呼んで確かめる。遅さそのものは別の Issue で扱う。
	t.Run("GET /api/admin/overview（SQL を直接呼んで比べる）", func(t *testing.T) {
		node := send(t, env.node, parityRequest{method: "GET", path: "/api/admin/overview", cookie: admin})
		if node.status != 200 {
			t.Fatalf("Node の status = %d", node.status)
		}
		o, err := (&sqlAdminUserStore{db: fx.db}).overview(context.Background(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := stdjson.Marshal(o)
		var gon any
		stdjson.Unmarshal(raw, &gon)
		if diffs := diffJSON("$", node.body, gon); len(diffs) > 0 {
			t.Fatalf("食い違い（Node → Go）:\n  %s", strings.Join(diffs, "\n  "))
		}
	})

	for _, path := range []string{
		"/api/admin/users", "/api/admin/users?kind=seed", "/api/admin/users?kind=sim&page=2", "/api/admin/users?kind=demo",
		"/api/admin/users?q=test-admin", "/api/admin/users?q=%20TEST-ADMIN%20", "/api/admin/users?q=%25", "/api/admin/users?q=_",
		"/api/admin/users?page=10000", "/api/admin/users?page=1e1", "/api/admin/users?page=%203%20", "/api/admin/users?page=0x2",
		"/api/admin/users?kind=x", "/api/admin/users?kind=real&kind=sim", "/api/admin/users?q=a&q=b",
		"/api/admin/users?q=" + strings.Repeat("a", 192), "/api/admin/users?page=abc", "/api/admin/users?page=0",
		"/api/admin/users?page=1.5", "/api/admin/users?page=10001", "/api/admin/users?page=Infinity",
		"/api/admin/users?page=1&page=2", "/api/admin/users?page=9007199254740993", "/api/admin/users?page=",
	} {
		same("GET "+path, "GET", path, admin, "", nil)
	}

	// 操作できない相手と、不正な入力。DB は変わらない
	for _, c := range []struct {
		name, method, path, body string
		header                   map[string]string
	}{
		{"いない相手の停止", "POST", "/api/admin/users/no-such-user/ban", "", nil},
		{"いない相手の解除", "POST", "/api/admin/users/no-such-user/unban", "", nil},
		{"いない相手の削除", "DELETE", "/api/admin/users/no-such-user", `{"email":"a@example.test"}`, json},
		{"自分自身の停止", "POST", "/api/admin/users/" + adminID + "/ban", "", nil},
		{"他の管理者の停止", "POST", "/api/admin/users/" + otherAdminID + "/ban", "", nil},
		{"他の管理者の削除", "DELETE", "/api/admin/users/" + otherAdminID, `{"email":"x"}`, json},
		{"停止に受け付けない本文", "POST", "/api/admin/users/no-such-user/ban", "x", map[string]string{"Content-Type": "text/html"}},
		{"停止に壊れた JSON（本文なし扱い）", "POST", "/api/admin/users/no-such-user/ban", "{a", json},
		{"削除の本文なし", "DELETE", "/api/admin/users/no-such-user", "", nil},
		{"削除の本文が空のオブジェクト", "DELETE", "/api/admin/users/no-such-user", `{}`, json},
		{"削除のメールが空", "DELETE", "/api/admin/users/no-such-user", `{"email":""}`, json},
		{"削除のメールが数", "DELETE", "/api/admin/users/no-such-user", `{"email":1}`, json},
		{"削除のメールが長すぎる", "DELETE", "/api/admin/users/no-such-user", `{"email":"` + strings.Repeat("a", 192) + `"}`, json},
		{"削除の本文が配列", "DELETE", "/api/admin/users/no-such-user", `[]`, json},
		{"削除の本文が text/plain", "DELETE", "/api/admin/users/no-such-user", `{"email":"a"}`, map[string]string{"Content-Type": "text/plain"}},
		{"書き込みの無いメソッド", "PUT", "/api/admin/users/no-such-user", `{}`, json},
		// ID が100文字を超えるものは比べない。Node は Fastify の maxParamLength（既定 100）に当たって、Zod より先に
		// 414（Fastify の形の本文）を返す。Go は各ルートの規則（191文字まで）で判定する。正しい ID は32文字で、
		// 画面の操作では起きない。
	} {
		same(c.name, c.method, c.path, admin, c.body, c.header)
	}

	// 停止・解除・削除の成功。同じ形の相手を Node 用・Go 用に1人ずつ作って比べる
	t.Run("停止→もう一度停止→解除→メール違い→削除", func(t *testing.T) {
		nodeID, nodeEmail := fx.user("user")
		goID, goEmail := fx.user("user")
		nodeCookie, goCookie := fx.session(nodeID, false), fx.session(goID, false)
		fx.session(nodeID, false)
		fx.session(goID, false)
		fx.studyLogs(nodeID, 2)
		fx.studyLogs(goID, 2)
		on := func(method, path, body string, header map[string]string) (parityRequest, parityRequest) {
			return parityRequest{method: method, path: strings.ReplaceAll(path, "{id}", nodeID), cookie: admin, body: strings.ReplaceAll(body, "{email}", nodeEmail), header: header},
				parityRequest{method: method, path: strings.ReplaceAll(path, "{id}", goID), cookie: admin, body: strings.ReplaceAll(body, "{email}", goEmail), header: header}
		}

		n, g := on("POST", "/api/admin/users/{id}/ban", "", nil)
		res := compareAdmin(t, env, n, g, "id", "email", "bannedAt")
		if res.status != 200 {
			t.Fatalf("停止 status = %d", res.status)
		}
		for _, id := range []string{nodeID, goID} {
			if fx.count("SELECT COUNT(*) FROM `user` WHERE id = ? AND bannedAt IS NOT NULL", id) != 1 ||
				fx.count("SELECT COUNT(*) FROM session WHERE userId = ?", id) != 0 {
				t.Fatalf("%s: 停止の印か session の削除が DB に無い", id)
			}
		}
		// 分担：Go が消した session の Cookie は、Node でも使えない（今の画面が落ちる）。次のログインを Node が
		// 断ることは apps/api/src/auth.banned-login.test.ts が確かめる。
		// Node の業務の API は Go へ移って消えていくので、Node に残る Better Auth のログイン必須の入口で見る。
		for _, cookie := range []string{nodeCookie, goCookie} {
			if r := fetch(t, env.node, "GET", "/api/auth/list-sessions", cookie); r.status != 401 {
				t.Errorf("停止した人の Cookie で Node が %d を返した", r.status)
			}
		}

		// 押し直しても最初に止めた日時のまま。消す session はもう無い
		var before string
		fx.db.QueryRow("SELECT bannedAt FROM `user` WHERE id = ?", goID).Scan(&before)
		n, g = on("POST", "/api/admin/users/{id}/ban", "", nil)
		compareAdmin(t, env, n, g, "id", "email", "bannedAt")
		var after string
		fx.db.QueryRow("SELECT bannedAt FROM `user` WHERE id = ?", goID).Scan(&after)
		if before != after {
			t.Errorf("押し直しで bannedAt が変わった: %s → %s", before, after)
		}

		n, g = on("POST", "/api/admin/users/{id}/unban", "", nil)
		compareAdmin(t, env, n, g, "id", "email")
		if fx.count("SELECT COUNT(*) FROM `user` WHERE id IN (?, ?) AND bannedAt IS NULL", nodeID, goID) != 2 {
			t.Fatal("解除が DB に無い")
		}

		n, g = on("DELETE", "/api/admin/users/{id}", `{"email":"someone-else@example.test"}`, json)
		compareAdmin(t, env, n, g)
		// 大文字と前後の空白は無視して比べる
		n, g = on("DELETE", "/api/admin/users/{id}", `{"email":"  {email}  "}`, json)
		n.body = strings.Replace(n.body, nodeEmail, strings.ToUpper(nodeEmail), 1)
		g.body = strings.Replace(g.body, goEmail, strings.ToUpper(goEmail), 1)
		compareAdmin(t, env, n, g, "id", "email")
		if fx.count("SELECT COUNT(*) FROM `user` WHERE id IN (?, ?)", nodeID, goID) != 0 ||
			fx.count("SELECT COUNT(*) FROM StudyLog WHERE userId IN (?, ?)", nodeID, goID) != 0 {
			t.Fatal("削除が DB に無い（学習記録も CASCADE で消えるはず）")
		}
	})
}
