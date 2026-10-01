//go:build parity

// マスター編集（JUK-78）を本物の DB に流し、Node と Go の応答を比べる。parity.sh から動かす。
//
// 大学名と ISBN は一意なので、作る・書き換える・消すは、Node 用と Go 用に名前だけ違う同じ形の操作を
// それぞれに送って応答を比べる（id・名前など相手ごとに違う値は、有るか無いかだけを見る）。
// 作った行はテストの最後に消す。
package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// maskIn は "faculties.*.id" のような場所の値を「有る」という印に置き換える。* は配列の全要素。
func maskIn(body any, path string) {
	key, rest, nested := strings.Cut(path, ".")
	switch v := body.(type) {
	case []any:
		if key == "*" {
			for _, item := range v {
				if nested {
					maskIn(item, rest)
				}
			}
		}
	case map[string]any:
		if !nested {
			maskKey(v, key)
			return
		}
		maskIn(v[key], rest)
	}
}

// comparePair は Node と Go にそれぞれのリクエストを送り、masked の場所は有るか無いかだけを比べる。
func comparePair(t *testing.T, env parityEnvironment, node, gon parityRequest, masked ...string) response {
	t.Helper()
	a, b := send(t, env.node, node), send(t, env.goURL, gon)
	for _, path := range masked {
		maskIn(a.body, path)
		maskIn(b.body, path)
	}
	if diffs := compareResponses(a, b); len(diffs) > 0 {
		t.Fatalf("%s %s の食い違い（Node → Go）:\n  %s", gon.method, gon.path, strings.Join(diffs, "\n  "))
	}
	return b
}

func TestParityAdminMasters(t *testing.T) {
	env := parityEnv(t)
	fx, db := newAdminFixture(t)
	adminID, _ := fx.user("admin")
	admin := fx.session(adminID, true)
	jsonHeader := map[string]string{"Content-Type": "application/json"}

	// このテストで作る大学・参考書の名前は、この印で始める（最後にまとめて消す）。
	prefix := "parity-" + randomHex(4)
	t.Cleanup(func() {
		db.Exec(`DELETE g FROM FinalGoal g JOIN Faculty f ON f.id = g.facultyId JOIN University u ON u.id = f.universityId
		         WHERE u.name LIKE ?`, prefix+"%")
		db.Exec("DELETE FROM University WHERE name LIKE ?", prefix+"%")
		db.Exec("DELETE t FROM Textbook t JOIN TextbookMaster tm ON tm.id = t.masterId WHERE tm.name LIKE ?", prefix+"%")
		db.Exec("DELETE FROM TextbookMaster WHERE name LIKE ?", prefix+"%")
	})

	same := func(name, method, path, cookie, body string, header map[string]string) {
		t.Run(name, func(t *testing.T) {
			pr := parityRequest{method: method, path: path, cookie: cookie, body: body, header: header}
			comparePair(t, env, pr, pr)
		})
	}

	// 管理者以外は、どのルートもハンドラまで来ない（未ログイン 401・一般 403・2段階認証なし 403）
	noTwoFactor := fx.session(adminID, false)
	for _, r := range [][2]string{
		{"GET", "/api/admin/universities"}, {"POST", "/api/admin/universities"}, {"GET", "/api/admin/universities/1"},
		{"PATCH", "/api/admin/universities/1"}, {"DELETE", "/api/admin/universities/1"}, {"GET", "/api/admin/tags"},
		{"POST", "/api/admin/faculties"}, {"PATCH", "/api/admin/faculties/1"}, {"DELETE", "/api/admin/faculties/1"},
		{"GET", "/api/admin/textbook-masters"}, {"POST", "/api/admin/textbook-masters"},
		{"PATCH", "/api/admin/textbook-masters/1"}, {"DELETE", "/api/admin/textbook-masters/1"},
	} {
		for _, c := range []struct{ who, cookie string }{
			{"未ログイン", ""}, {"一般の利用者", env.cookiesFor(firstUser)[0]}, {"2段階認証なし", noTwoFactor},
		} {
			same(fmt.Sprintf("%s %s（%s）", r[0], r[1], c.who), r[0], r[1], c.cookie, `{}`, jsonHeader)
		}
	}

	// 読み取り。同じ DB を同じ規則で読む
	var universityWithFaculties int64
	if err := db.QueryRow("SELECT universityId FROM Faculty ORDER BY id LIMIT 1").Scan(&universityWithFaculties); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/admin/universities", "/api/admin/universities?q=%E5%A4%A7%E5%AD%A6", "/api/admin/universities?q=%20%E6%9D%B1%E4%BA%AC%20",
		"/api/admin/universities?q=%25", "/api/admin/universities?q=_", "/api/admin/universities?q=%20%20", "/api/admin/universities?page=2",
		"/api/admin/universities?page=1000", "/api/admin/universities?page=1e1", "/api/admin/universities?page=0x2",
		"/api/admin/universities?q=a&q=b", "/api/admin/universities?q=" + strings.Repeat("a", 101), "/api/admin/universities?page=0",
		"/api/admin/universities?page=1001", "/api/admin/universities?page=abc", "/api/admin/universities?page=1.5",
		fmt.Sprintf("/api/admin/universities/%d", universityWithFaculties), "/api/admin/universities/999999999",
		"/api/admin/universities/0", "/api/admin/universities/abc", "/api/admin/universities/01",
		"/api/admin/tags",
		"/api/admin/textbook-masters", "/api/admin/textbook-masters?q=978", "/api/admin/textbook-masters?q=%E8%8B%B1",
		"/api/admin/textbook-masters?q=%20", "/api/admin/textbook-masters?q=a&q=b",
	} {
		same("GET "+path, "GET", path, admin, "", nil)
	}

	// 不正な入力・無い相手・重なり。DB は変わらない
	var existingUniversity, existingISBN string
	var tag1, tag2 int64
	if err := db.QueryRow("SELECT name FROM University ORDER BY id LIMIT 1").Scan(&existingUniversity); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT isbn FROM TextbookMaster ORDER BY id LIMIT 1").Scan(&existingISBN); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT MIN(id), MAX(id) FROM Tag").Scan(&tag1, &tag2); err != nil || tag1 == tag2 {
		t.Fatalf("タグが2つ以上要る: %v", err)
	}
	metrics := `[{"unit":"page","totalAmount":300,"isDefault":true},{"unit":"chapter","totalAmount":12,"isDefault":false}]`
	textbook := func(name, isbn string) string {
		return fmt.Sprintf(`{"name":%q,"publisher":" 社 ","edition":"","isbn":%q,"metrics":%s}`, name, isbn, metrics)
	}
	counts := func() [4]int {
		var c [4]int
		for i, table := range []string{"University", "Faculty", "_FacultyToTag", "TextbookMaster"} {
			c[i] = fx.count("SELECT COUNT(*) FROM " + table)
		}
		return c
	}
	before := counts()
	for _, c := range []struct{ name, method, path, body string }{
		{"大学：本文なし", "POST", "/api/admin/universities", ""},
		{"大学：空のオブジェクト", "POST", "/api/admin/universities", `{}`},
		{"大学：名前が空白だけ", "POST", "/api/admin/universities", `{"name":"  ","prefecture":"東京都","type":"国立"}`},
		{"大学：名前が101文字", "POST", "/api/admin/universities", `{"name":"` + strings.Repeat("a", 101) + `","prefecture":"東京都","type":"国立"}`},
		{"大学：都道府県でない", "POST", "/api/admin/universities", `{"name":"a","prefecture":"東京","type":"国立"}`},
		{"大学：種別が数", "POST", "/api/admin/universities", `{"name":"a","prefecture":"東京都","type":1}`},
		{"大学：名前が重なる", "POST", "/api/admin/universities", fmt.Sprintf(`{"name":%q,"prefecture":"東京都","type":"国立"}`, existingUniversity)},
		{"大学：無い大学の書き換え", "PATCH", "/api/admin/universities/999999999", `{"name":"a","prefecture":"東京都","type":"国立"}`},
		{"大学：ID の形が違う書き換え", "PATCH", "/api/admin/universities/0", `{}`},
		{"大学：無い大学の削除", "DELETE", "/api/admin/universities/999999999", ""},
		{"大学：ID の形が違う削除", "DELETE", "/api/admin/universities/x", ""},
		{"学部：受験日の日が32", "POST", "/api/admin/faculties", fmt.Sprintf(`{"name":"a","examDate":"2027-02-32","tagIds":[],"universityId":%d}`, universityWithFaculties)},
		{"学部：受験日の形", "POST", "/api/admin/faculties", `{"name":"a","examDate":"2027-2-1","tagIds":[],"universityId":1}`},
		{"学部：タグが21個", "POST", "/api/admin/faculties", `{"name":"a","examDate":"2027-02-01","tagIds":[` + strings.TrimSuffix(strings.Repeat("1,", 21), ",") + `],"universityId":1}`},
		{"学部：タグが重なる", "POST", "/api/admin/faculties", `{"name":"a","examDate":"2027-02-01","tagIds":[1,1],"universityId":1}`},
		{"学部：大学が0", "POST", "/api/admin/faculties", `{"name":"a","examDate":"2027-02-01","tagIds":[],"universityId":0}`},
		{"学部：無い大学", "POST", "/api/admin/faculties", `{"name":"a","examDate":"2027-02-01","tagIds":[],"universityId":999999999}`},
		{"学部：無いタグ", "POST", "/api/admin/faculties", fmt.Sprintf(`{"name":%q,"examDate":"2027-02-01","tagIds":[%d,999999999],"universityId":%d}`, prefix, tag1, universityWithFaculties)},
		{"学部：無い学部の書き換え", "PATCH", "/api/admin/faculties/999999999", `{"name":"a","examDate":"2027-02-01","tagIds":[]}`},
		{"学部：無い学部の削除", "DELETE", "/api/admin/faculties/999999999", ""},
		{"参考書：名前なし", "POST", "/api/admin/textbook-masters", `{"isbn":"9784000000000","metrics":[]}`},
		{"参考書：ISBN が3桁", "POST", "/api/admin/textbook-masters", textbook("a", "123")},
		{"参考書：既定が無い", "POST", "/api/admin/textbook-masters", `{"name":"a","isbn":"9784000000000","metrics":[{"unit":"page","totalAmount":1,"isDefault":false}]}`},
		{"参考書：単位が重なる", "POST", "/api/admin/textbook-masters", `{"name":"a","isbn":"9784000000000","metrics":[{"unit":"page","totalAmount":1,"isDefault":true},{"unit":"page","totalAmount":2,"isDefault":false}]}`},
		{"参考書：総量が小数", "POST", "/api/admin/textbook-masters", `{"name":"a","isbn":"9784000000000","metrics":[{"unit":"page","totalAmount":1.5,"isDefault":true}]}`},
		{"参考書：ISBN が重なる", "POST", "/api/admin/textbook-masters", textbook(prefix, existingISBN)},
		{"参考書：無い参考書の書き換え", "PATCH", "/api/admin/textbook-masters/999999999", textbook(prefix, "9784000000000")},
		{"参考書：無い参考書の削除", "DELETE", "/api/admin/textbook-masters/999999999", ""},
	} {
		header := jsonHeader
		if c.body == "" {
			header = nil
		}
		same(c.name, c.method, c.path, admin, c.body, header)
	}
	for _, c := range []struct{ name, method, path, body, contentType string }{
		{"受け付けない Content-Type", "POST", "/api/admin/universities", `{}`, "text/html"},
		{"text/plain の JSON は文字列として読む", "POST", "/api/admin/universities", `{"name":"a","prefecture":"東京都","type":"国立"}`, "text/plain"},
		{"壊れた JSON は本文なし扱い", "PATCH", "/api/admin/faculties/1", `{a`, "application/json"},
		{"削除に受け付けない本文", "DELETE", "/api/admin/textbook-masters/999999999", `x`, "text/html"},
	} {
		same(c.name, c.method, c.path, admin, c.body, map[string]string{"Content-Type": c.contentType})
	}
	same("書き込みの無いメソッド", "PUT", "/api/admin/universities/1", admin, `{}`, jsonHeader)
	for _, r := range [][2]string{{"GET", "/api/admin/tags/"}, {"GET", "/api/admin/universities/1/x"}} {
		same("余分なパス "+r[0]+" "+r[1], r[0], r[1], admin, `{}`, jsonHeader)
	}
	// ID が空のパス（/api/admin/universities/ など）は比べない。Node（Fastify）は :id に空文字を当てて 400
	// （Zod の invalid_format）を返し、Go の ServeMux は {id} に空を当てないので 404 になる。画面の操作では起きず、
	// 先に Go へ移した {id} のルート（/api/universities/{id}・/api/study-logs/{id} など）も同じ違いを持っている。
	if after := counts(); after != before {
		t.Fatalf("断ったはずの操作で行が変わった: %v → %v", before, after)
	}

	t.Run("大学と学部：作る→重なり→書き換え→学部→使われていると消せない→消す", func(t *testing.T) {
		nodeName, goName := prefix+"-node", prefix+"-go"
		// pair は Node 用と Go 用のリクエストを作る。{name}・{uid}・{fid} を、それぞれの名前・大学・学部にする。
		ids := map[string]map[string]string{"node": {"{name}": nodeName}, "go": {"{name}": goName}}
		pair := func(method, path, body string) (parityRequest, parityRequest) {
			build := func(side string) parityRequest {
				p, b := path, body
				for k, v := range ids[side] {
					p, b = strings.ReplaceAll(p, k, v), strings.ReplaceAll(b, k, v)
				}
				var header map[string]string
				if b != "" {
					header = jsonHeader
				}
				return parityRequest{method: method, path: p, cookie: admin, body: b, header: header}
			}
			return build("node"), build("go")
		}
		step := func(method, path, body string, wantStatus int, masked ...string) {
			t.Helper()
			n, g := pair(method, path, body)
			if res := comparePair(t, env, n, g, masked...); res.status != wantStatus {
				t.Fatalf("%s %s: status = %d, want %d（%v）", method, path, res.status, wantStatus, res.body)
			}
		}
		idOf := func(query string, args ...any) string {
			t.Helper()
			var id int64
			if err := db.QueryRow(query, args...).Scan(&id); err != nil {
				t.Fatal(err)
			}
			return fmt.Sprint(id)
		}
		// 大学を探す画面（Go）の一覧に、その名前の大学が出るか。キャッシュを捨てられているかを見る。
		listed := func(name string) bool {
			t.Helper()
			res := fetch(t, env.goURL, "GET", "/api/universities", admin)
			return slices.ContainsFunc(res.body.([]any), func(u any) bool { return u.(map[string]any)["name"] == name })
		}
		if listed(goName) {
			t.Fatal("作る前から一覧にある")
		}

		step("POST", "/api/admin/universities", `{"name":" {name} ","prefecture":"北海道","type":"私立"}`, 201, "id", "name")
		for side, name := range map[string]string{"node": nodeName, "go": goName} {
			ids[side]["{uid}"] = idOf("SELECT id FROM University WHERE name = ?", name)
		}
		if !listed(goName) {
			t.Fatal("Go で作った大学が、大学を探す画面の一覧にすぐ出ない（キャッシュを捨てていない）")
		}
		step("POST", "/api/admin/universities", `{"name":"{name}","prefecture":"北海道","type":"私立"}`, 409)
		step("PATCH", "/api/admin/universities/{uid}", `{"name":"{name}","prefecture":"東京都","type":"国立"}`, 200, "id", "name")
		// 相手の名前に書き換えようとすると重なる
		ids["node"]["{other}"], ids["go"]["{other}"] = goName, nodeName
		step("PATCH", "/api/admin/universities/{uid}", `{"name":"{other}","prefecture":"東京都","type":"国立"}`, 409)

		faculty := fmt.Sprintf(`{"name":"法学部","examDate":"2027-02-31","tagIds":[%d,%d],"universityId":{uid}}`, tag2, tag1)
		step("POST", "/api/admin/faculties", faculty, 201, "id", "universityId")
		step("POST", "/api/admin/faculties", faculty, 409)
		for side := range ids {
			ids[side]["{fid}"] = idOf("SELECT id FROM Faculty WHERE universityId = ?", ids[side]["{uid}"])
		}
		step("GET", "/api/admin/universities/{uid}", "", 200, "university.id", "university.name", "faculties.*.id")
		step("PATCH", "/api/admin/faculties/{fid}", fmt.Sprintf(`{"name":" 経済学部 ","examDate":"2028-01-15","tagIds":[%d]}`, tag2), 200, "id", "universityId")
		step("GET", "/api/admin/universities?q={name}", "", 200, "universities.*.id", "universities.*.name")

		// 志望校に使われている学部・大学は消せない
		userID, _ := fx.user("user")
		for side := range ids {
			if _, err := db.Exec("INSERT INTO FinalGoal (facultyId, userId) VALUES (?, ?)", ids[side]["{fid}"], userID); err != nil {
				t.Fatal(err)
			}
		}
		step("DELETE", "/api/admin/faculties/{fid}", "", 409)
		step("DELETE", "/api/admin/universities/{uid}", "", 409)
		if _, err := db.Exec("DELETE FROM FinalGoal WHERE userId = ?", userID); err != nil {
			t.Fatal(err)
		}

		step("DELETE", "/api/admin/faculties/{fid}", "", 204)
		step("DELETE", "/api/admin/faculties/{fid}", "", 404)
		step("DELETE", "/api/admin/universities/{uid}", "", 204)
		step("GET", "/api/admin/universities/{uid}", "", 404)
		if listed(goName) {
			t.Fatal("Go で消した大学が、大学を探す画面の一覧に残っている（キャッシュを捨てていない）")
		}
	})

	t.Run("参考書：作る→重なり→書き換え→検索→使われていると消せない→消す", func(t *testing.T) {
		// ISBN は13桁の数字。ハイフンは API が取り除く
		stamp := fmt.Sprintf("%08d", time.Now().UnixNano()%1e8)
		sides := map[string][2]string{ // 名前・ISBN
			"node": {prefix + "-node", "979-1" + stamp + "-1"},
			"go":   {prefix + "-go", "979-1" + stamp + "-2"},
		}
		masterIDs := map[string]string{}
		req := func(side, method, path, body string) parityRequest {
			r := strings.NewReplacer("{name}", sides[side][0], "{isbn}", sides[side][1], "{mid}", masterIDs[side])
			var header map[string]string
			if body != "" {
				header = jsonHeader
			}
			return parityRequest{method: method, path: r.Replace(path), cookie: admin, body: r.Replace(body), header: header}
		}
		step := func(method, path, body string, wantStatus int, masked ...string) {
			t.Helper()
			if res := comparePair(t, env, req("node", method, path, body), req("go", method, path, body), masked...); res.status != wantStatus {
				t.Fatalf("%s %s: status = %d, want %d（%v）", method, path, res.status, wantStatus, res.body)
			}
		}

		step("POST", "/api/admin/textbook-masters", textbook("{name}", "{isbn}"), 201, "id", "name", "isbn")
		for side, v := range sides {
			var id int64
			if err := db.QueryRow("SELECT id FROM TextbookMaster WHERE name = ?", v[0]).Scan(&id); err != nil {
				t.Fatal(err)
			}
			masterIDs[side] = fmt.Sprint(id)
		}
		step("POST", "/api/admin/textbook-masters", textbook("{name}", "{isbn}"), 409)
		step("PATCH", "/api/admin/textbook-masters/{mid}",
			`{"name":"{name}","publisher":null,"edition":" 第3版 ","isbn":"{isbn}","metrics":[{"unit":"question","totalAmount":450,"isDefault":true}]}`,
			200, "id", "name", "isbn")
		step("GET", "/api/admin/textbook-masters?q={name}", "", 200, "*.id", "*.name", "*.isbn")

		// 利用者の参考書が使っていれば消せない
		userID, _ := fx.user("user")
		for side, v := range sides {
			if _, err := db.Exec("INSERT INTO Textbook (userId, name, masterId, updatedAt) VALUES (?, ?, ?, ?)", userID, v[0], masterIDs[side], time.Now()); err != nil {
				t.Fatal(err)
			}
		}
		step("DELETE", "/api/admin/textbook-masters/{mid}", "", 409)
		if _, err := db.Exec("DELETE FROM Textbook WHERE userId = ?", userID); err != nil {
			t.Fatal(err)
		}
		step("DELETE", "/api/admin/textbook-masters/{mid}", "", 204)
		step("DELETE", "/api/admin/textbook-masters/{mid}", "", 404)
		if fx.count("SELECT COUNT(*) FROM TextbookMasterMetric WHERE masterId IN (?, ?)", masterIDs["node"], masterIDs["go"]) != 0 {
			t.Fatal("総量の候補が CASCADE で消えていない")
		}
	})
}
