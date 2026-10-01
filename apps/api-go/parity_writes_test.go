//go:build parity

// 書き込みの API の応答一致テスト（JUK-75）。parity_test.go と同じく parity.sh から動かす。
//
// Node と Go は同じ DB を使うので、成功する書き込みは Node → Go の順に同じ本文を送り、
// 2回目（Go）の応答が1回目（Node）と同じかを比べる。どちらも同じ値で上書きするので、比べる時点の
// DB の状態は揃っている。書き換えた行は、テストの前に控えておいた値へ最後に戻す。
package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

const jsonType = "application/json"

// writeCases が比べる書き込み。どれも入力チェックや持ち主の確認で弾くもので、DB を変えないので全員に送る。
// 成功する書き込みは、下の Test…Scenario が手順を決めて比べる。
var userWriteCases = []struct {
	method      string
	path        string
	contentType string // "" なら付けない
	body        string
	as          []who
	// ignore は値が毎回変わる項目（書き込んだ時刻）。有るか無いかだけを比べる。
	ignore []string
}{
	// 学習予定（JUK-75）。弾かれるものだけ（成功する書き込みは TestParityStudyPlanScenario）
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[{"content":"a"}]}`, []who{anonymous, forged, unknownUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01"}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":{}}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[1]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[{}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[{"content":"   "}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[{},{"textbookId":0}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[{"content":"a"},{"textbookId":1.5}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[{"rangeStart":1}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[{"rangeUnit":"x"}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"x","items":[{"textbookId":0}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":" ","items":[{"content":"a"}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2026-02-30","items":[{"content":"a"}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", jsonType, `{"date":"2099-10-01","items":[{"textbookId":999999999}]}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans", "text/html", `x`, []who{eachUser}, nil},
	{"PATCH", "/api/study-plans/abc", jsonType, `{}`, []who{anonymous, eachUser}, nil},
	{"PATCH", "/api/study-plans/999999999", jsonType, `{"date":""}`, []who{eachUser}, nil},
	{"PATCH", "/api/study-plans/999999999", jsonType, `{"done":"yes"}`, []who{eachUser}, nil},
	{"PATCH", "/api/study-plans/999999999", jsonType, `{"rangeEnd":3}`, []who{eachUser}, nil},
	{"PATCH", "/api/study-plans/999999999", jsonType, `{}`, []who{eachUser}, nil},
	{"DELETE", "/api/study-plans/abc", "", ``, []who{eachUser}, nil},
	{"DELETE", "/api/study-plans/999999999", "", ``, []who{eachUser}, nil},
	{"POST", "/api/study-plans/abc/complete", jsonType, `{"minutes":30}`, []who{anonymous, eachUser}, nil},
	{"POST", "/api/study-plans/999999999/complete", jsonType, `{}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans/999999999/complete", jsonType, `{"minutes":30,"rangeStart":1}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans/999999999/complete", jsonType, `{"minutes":30,"memo":null}`, []who{eachUser}, nil},
	{"POST", "/api/study-plans/999999999/complete", jsonType, `{"minutes":30}`, []who{eachUser}, nil},
	// 無いメソッドは 404
	{"GET", "/api/study-plans/1", "", ``, []who{eachUser}, nil},
	{"PUT", "/api/study-plans/1", jsonType, `{}`, []who{eachUser}, nil},
	{"GET", "/api/study-plans/1/complete", "", ``, []who{eachUser}, nil},

	// 志望校（JUK-75）。弾かれるものだけ（成功する書き込みは TestParityGoalScenario）
	{"POST", "/api/goals", jsonType, `{"facultyId":1}`, []who{anonymous, forged, unknownUser}, nil},
	{"POST", "/api/goals", jsonType, `{}`, []who{eachUser}, nil},
	{"POST", "/api/goals", jsonType, `{"facultyId":"1"}`, []who{eachUser}, nil},
	{"POST", "/api/goals", jsonType, `{"facultyId":0}`, []who{eachUser}, nil},
	{"POST", "/api/goals", jsonType, `{"facultyId":1.5}`, []who{eachUser}, nil},
	{"POST", "/api/goals", jsonType, `{"facultyId":1e20}`, []who{eachUser}, nil},
	{"POST", "/api/goals", jsonType, `{"facultyId":1,"status":"x"}`, []who{eachUser}, nil},
	{"POST", "/api/goals", jsonType, `{"facultyId":1,"status":null}`, []who{eachUser}, nil},
	{"POST", "/api/goals", jsonType, `null`, []who{eachUser}, nil},
	{"POST", "/api/goals", "text/html", `x`, []who{eachUser}, nil},
	{"PUT", "/api/goals/abc", jsonType, `{}`, []who{anonymous, eachUser}, nil},
	{"PUT", "/api/goals/999999999", jsonType, `{"facultyId":-1}`, []who{eachUser}, nil},
	{"PUT", "/api/goals/999999999", jsonType, `{"status":1}`, []who{eachUser}, nil},
	{"PUT", "/api/goals/999999999", jsonType, `{}`, []who{eachUser}, nil},
	{"PATCH", "/api/goals/abc", jsonType, `{}`, []who{eachUser}, nil},
	{"PATCH", "/api/goals/first-choice", jsonType, `{}`, []who{eachUser}, nil},
	{"PATCH", "/api/goals/999999999", jsonType, `{"isFirstChoice":"y"}`, []who{eachUser}, nil},
	{"PATCH", "/api/goals/999999999", jsonType, `{"note":"` + strings.Repeat("a", 501) + `"}`, []who{eachUser}, nil},
	{"PATCH", "/api/goals/999999999", jsonType, `{"note":null}`, []who{eachUser}, nil},
	{"DELETE", "/api/goals/abc", "", ``, []who{anonymous, eachUser}, nil},
	{"DELETE", "/api/goals/first-choice", "", ``, []who{eachUser}, nil},
	{"DELETE", "/api/goals/999999999", "", ``, []who{eachUser}, nil},
	// 無いメソッドは 404
	{"GET", "/api/goals/1", "", ``, []who{eachUser}, nil},
	{"POST", "/api/goals/1", jsonType, `{}`, []who{eachUser}, nil},
	{"POST", "/api/goals/first-choice", jsonType, `{}`, []who{eachUser}, nil},

	// 参考書（JUK-75）。弾かれるものだけ（成功する書き込みは TestParityTextbookScenario）
	{"POST", "/api/textbooks", jsonType, `{"name":"a"}`, []who{anonymous, forged, unknownUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":""}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":"　"}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":"` + strings.Repeat("あ", 101) + `"}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":1}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":"a","subject":"x"}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":"a","rangeUnit":null}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":"","subject":"x"}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"masterId":0}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"masterId":"1"}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"masterId":1.5}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"masterId":1e20}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"masterId":0,"name":1}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":"","masterId":0}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"name":"","masterId":1.5}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `{"masterId":999999999}`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", jsonType, `[]`, []who{eachUser}, nil},
	{"POST", "/api/textbooks", "text/html", `x`, []who{eachUser}, nil},
	{"PATCH", "/api/textbooks/abc", jsonType, `{}`, []who{anonymous, eachUser}, nil},
	{"PATCH", "/api/textbooks/999999999", jsonType, `{"totalAmount":0}`, []who{eachUser}, nil},
	{"PATCH", "/api/textbooks/999999999", jsonType, `{"totalAmount":1.5}`, []who{eachUser}, nil},
	{"PATCH", "/api/textbooks/999999999", jsonType, `{"totalAmount":100001}`, []who{eachUser}, nil},
	{"PATCH", "/api/textbooks/999999999", jsonType, `{"rangeUnit":null}`, []who{eachUser}, nil},
	{"PATCH", "/api/textbooks/999999999", jsonType, `{"targetDate":"2026-02-30"}`, []who{eachUser}, nil},
	{"PATCH", "/api/textbooks/999999999", jsonType, `{"targetDate":"1900-02-29"}`, []who{eachUser}, nil},
	{"PATCH", "/api/textbooks/999999999", jsonType, `{"subject":"x"}`, []who{eachUser}, nil},
	{"PATCH", "/api/textbooks/999999999", jsonType, `{}`, []who{eachUser}, nil},
	// 無いメソッドは 404
	{"GET", "/api/textbooks/1", "", ``, []who{eachUser}, nil},
	{"DELETE", "/api/textbooks/1", "", ``, []who{eachUser}, nil},
	{"PUT", "/api/textbooks", jsonType, `{}`, []who{eachUser}, nil},
	{"POST", "/api/textbook-masters", jsonType, `{}`, []who{eachUser}, nil},
}

func TestParityUserWrites(t *testing.T) {
	env := parityEnv(t)
	for _, c := range userWriteCases {
		for _, w := range c.as {
			name := c.body
			if len(name) > 60 {
				name = name[:60] + "…"
			}
			t.Run(fmt.Sprintf("%s %s %q（%s）", c.method, c.path, name, whoNames[w]), func(t *testing.T) {
				for i, cookie := range env.cookiesFor(w) {
					pr := parityRequest{method: c.method, path: c.path, cookie: cookie, body: c.body, header: map[string]string{}}
					if c.contentType != "" {
						pr.header["Content-Type"] = c.contentType
					}
					node, gon := send(t, env.node, pr), send(t, env.goURL, pr)
					for _, key := range c.ignore {
						maskKey(node.body, key)
						maskKey(gon.body, key)
					}
					if diffs := compareResponses(node, gon); len(diffs) > 0 {
						t.Fatalf("%d人目で食い違い（Node → Go）:\n  %s", i+1, strings.Join(diffs, "\n  "))
					}
				}
			})
		}
	}
}

// maskKey は、値が毎回変わる項目を「有る」という印に置き換える。
func maskKey(body any, key string) {
	if m, ok := body.(map[string]any); ok {
		if _, has := m[key]; has {
			m[key] = "(present)"
		}
	}
}

// sessionUserID は Cookie のセッションの利用者 ID。
func sessionUserID(t *testing.T, db *sql.DB, cookie string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", cookie)
	s, err := (&sessionAuth{db: db, secret: []byte(os.Getenv("BETTER_AUTH_SECRET"))}).load(req)
	if err != nil || s == nil {
		t.Fatalf("セッションを読めません: %v", err)
	}
	return s.UserID
}

// TestParityStudyPlanScenario は学習予定の成功する書き込みを比べる（JUK-75）。
//
// 同じ手順を Node と Go で1回ずつ流して各段を比べる。
// 手順が作る予定と実績は日付を 2099 年にして見分け、各回の終わりに消す。
func TestParityStudyPlanScenario(t *testing.T) {
	env := parityEnv(t)
	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cookie := env.cookiesFor(firstUser)[0]
	userID := sessionUserID(t, db, cookie)

	const marker = "2099-01-01" // これ以降の日付の予定・実績は、手順が作ったもの
	var existing int
	db.QueryRow(`SELECT (SELECT COUNT(*) FROM StudyPlan WHERE userId = ? AND date >= ?) +
		(SELECT COUNT(*) FROM StudyLog WHERE userId = ? AND date >= ?)`, userID, marker, userID, marker).Scan(&existing)
	if existing > 0 {
		t.Fatalf("1人目に 2099 年以降の予定・実績が %d 件あり、手順が作ったものと見分けられません", existing)
	}
	cleanup := func() {
		db.Exec("DELETE FROM StudyLog WHERE userId = ? AND date >= ?", userID, marker)
		db.Exec("DELETE FROM StudyPlan WHERE userId = ? AND date >= ?", userID, marker)
	}
	defer cleanup()

	var firstAt sql.NullString
	var updatedAt string
	if err := db.QueryRow("SELECT firstStudyLogAt, updatedAt FROM `user` WHERE id = ?", userID).Scan(&firstAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec("UPDATE `user` SET firstStudyLogAt = ?, updatedAt = ? WHERE id = ?", firstAt, updatedAt, userID); err != nil {
			t.Errorf("初回記録の印を戻せません: %v", err)
		}
	}()

	tbUnit, tbTotal := "chapter", int64(12)
	inserted, err := db.Exec(`INSERT INTO Textbook (userId, name, totalAmount, rangeUnit, createdAt, updatedAt)
		VALUES (?, '応答一致テストの参考書', ?, ?, UTC_TIMESTAMP(3), UTC_TIMESTAMP(3))`, userID, tbTotal, tbUnit)
	if err != nil {
		t.Fatal(err)
	}
	tbID, _ := inserted.LastInsertId()
	defer db.Exec("DELETE FROM Textbook WHERE id = ?", tbID)
	var otherTextbook, otherPlan int64
	if err := db.QueryRow("SELECT id FROM Textbook WHERE userId <> ? ORDER BY id LIMIT 1", userID).Scan(&otherTextbook); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT id FROM StudyPlan WHERE userId <> ? ORDER BY id LIMIT 1", userID).Scan(&otherPlan); err != nil {
		t.Fatal(err)
	}

	// planIDs は、その日付に手順が作った予定の ID（作った順）。
	planIDs := func(date string) []int64 {
		rows, err := db.Query("SELECT id FROM StudyPlan WHERE userId = ? AND date = ? ORDER BY id", userID, date)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			ids = append(ids, id)
		}
		return ids
	}

	run := func(base string) []response {
		defer cleanup()
		var out []response
		call := func(method, path, body string) response {
			t.Helper()
			pr := parityRequest{method: method, path: path, cookie: cookie, body: body, header: map[string]string{}}
			if body != "" {
				pr.header["Content-Type"] = jsonType
			}
			res := send(t, base, pr)
			for _, p := range []string{"id", "createdAt", "updatedAt",
				"log.id", "log.studyPlanId", "log.createdAt", "log.updatedAt", "plan.id", "plan.createdAt", "plan.updatedAt"} {
				maskPath(res.body, p)
			}
			out = append(out, res)
			return res
		}
		planPath := func(id int64, suffix string) string { return fmt.Sprintf("/api/study-plans/%d%s", id, suffix) }

		// 作成。同じ参考書を2つの予定に使っても、自分の参考書として数えるのは1つ
		call("POST", "/api/study-plans", fmt.Sprintf(`{"date":"2099-10-01","items":[{"content":" 応答一致 "},{"textbookId":%d,"rangeStart":1,"rangeEnd":3,"rangeUnit":%q,"subject":"math"},{"textbookId":%d,"content":"同じ参考書"}]}`, tbID, tbUnit, tbID))
		call("POST", "/api/study-plans", fmt.Sprintf(`{"date":"2099-10-02","items":[{"content":"a"},{"textbookId":%d}]}`, otherTextbook))
		ids := planIDs("2099-10-01")
		if len(ids) != 3 {
			t.Fatalf("作った予定が %d 件", len(ids))
		}
		p1, p2, p3 := ids[0], ids[1], ids[2]

		// 書き換え。送った項目だけが変わる
		call("PATCH", planPath(p1, ""), `{"content":"  x  ","subject":null}`)
		call("PATCH", planPath(p1, ""), `{"date":"2099-10-03","done":true}`)
		call("PATCH", planPath(p1, ""), `{"done":false}`)
		call("PATCH", planPath(p1, ""), `{}`)
		call("PATCH", planPath(p1, ""), fmt.Sprintf(`{"textbookId":%d,"rangeStart":2,"rangeEnd":4,"rangeUnit":"page"}`, tbID))
		call("PATCH", planPath(p1, ""), `{"textbookId":null,"rangeStart":null,"rangeEnd":null,"rangeUnit":null}`)
		call("PATCH", planPath(p1, ""), fmt.Sprintf(`{"textbookId":%d}`, otherTextbook))
		// ほかの人の予定：入力が正しければ 404、不正なら先に 400
		call("PATCH", planPath(otherPlan, ""), `{"content":"x"}`)
		call("PATCH", planPath(otherPlan, ""), `{"done":"yes"}`)

		// 完了。初めての記録なら isFirstStudyLog が true。範囲は送らなければ予定のもの
		if _, err := db.Exec("UPDATE `user` SET firstStudyLogAt = NULL WHERE id = ?", userID); err != nil {
			t.Fatal(err)
		}
		call("POST", planPath(p2, "/complete"), `{"minutes":30,"memo":"  完了  "}`)
		call("POST", planPath(p2, "/complete"), `{"minutes":30}`)
		call("PATCH", planPath(p2, ""), `{"done":false}`)
		// 範囲を送ると、予定の参考書の設定で確かめる（総量を超える・単位が違う・null で範囲なし）
		call("POST", planPath(p3, "/complete"), fmt.Sprintf(`{"minutes":5,"rangeStart":1,"rangeEnd":%d,"rangeUnit":%q}`, tbTotal+1, tbUnit))
		call("POST", planPath(p3, "/complete"), `{"minutes":5,"rangeStart":1,"rangeEnd":2,"rangeUnit":"page"}`)
		call("POST", planPath(p3, "/complete"), `{"minutes":5,"rangeStart":null,"rangeEnd":null}`)
		// 参考書の無い予定は、どの範囲でも通る
		call("POST", planPath(p1, "/complete"), `{"minutes":10,"rangeStart":1,"rangeEnd":2,"rangeUnit":"page"}`)
		call("POST", planPath(otherPlan, "/complete"), `{"minutes":10}`)

		// 削除。実績の付いた予定を消しても実績は残る（studyPlanId が null になる）
		call("DELETE", planPath(p2, ""), "")
		call("DELETE", planPath(p2, ""), "")
		call("DELETE", planPath(otherPlan, ""), "")
		var orphan int
		db.QueryRow("SELECT COUNT(*) FROM StudyLog WHERE userId = ? AND date >= ? AND studyPlanId IS NULL", userID, marker).Scan(&orphan)
		out = append(out, response{status: orphan})
		return out
	}

	node := run(env.node)
	gon := run(env.goURL)
	if len(node) != len(gon) {
		t.Fatalf("段の数が違う: Node %d、Go %d", len(node), len(gon))
	}
	// 両方が同じように失敗しても「一致」になるので、各段が狙いどおりの結果かも確かめる。
	// 最後の段はステータスではなく、予定を消した後に残った実績の数（1件）。
	want := []int{201, 400, 200, 200, 200, 200, 200, 200, 400, 404, 400,
		201, 409, 409, 400, 400, 201, 201, 404, 200, 404, 404, 1}
	for i := range node {
		if i < len(want) && node[i].status != want[i] {
			t.Errorf("%d段目：Node が %d（狙いは %d）: %v", i+1, node[i].status, want[i], node[i].body)
		}
		if diffs := compareResponses(node[i], gon[i]); len(diffs) > 0 {
			t.Errorf("%d段目で食い違い（Node → Go）:\n  %s", i+1, strings.Join(diffs, "\n  "))
		}
	}
}

// maskPath は "log.id" のような場所の値を「有る」という印に置き換える（無ければ何もしない）。
func maskPath(body any, path string) {
	keys := strings.Split(path, ".")
	for _, key := range keys[:len(keys)-1] {
		m, ok := body.(map[string]any)
		if !ok {
			return
		}
		body = m[key]
	}
	maskKey(body, keys[len(keys)-1])
}

// maskEach は配列の各要素の項目を「有る」という印に置き換える（一覧の id・日時は、作り直すたびに変わる）。
func maskEach(body any, keys ...string) {
	list, _ := body.([]any)
	for _, element := range list {
		for _, key := range keys {
			maskKey(element, key)
		}
	}
}

// TestParityGoalScenario は志望校の成功する書き込みを比べる（JUK-75）。
//
// 同じ手順（登録 → 重複 → 学部の差し替え → 第一志望・メモ → 削除）を Node と Go で1回ずつ流し、各段の応答を比べる。
// 第一志望の付け替えは1人目の既存の志望校も書き換えるので、手順の前に控えた値へ毎回戻す。
func TestParityGoalScenario(t *testing.T) {
	env := parityEnv(t)
	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cookie := env.cookiesFor(firstUser)[0]
	userID := sessionUserID(t, db, cookie)

	// 1人目の既存の志望校。手順の後に、第一志望・メモ・ステータスをこの値へ戻し、これより後に作った行を消す。
	type savedGoal struct {
		id            int64
		isFirstChoice bool
		note          sql.NullString
		status        string
	}
	var saved []savedGoal
	rows, err := db.Query("SELECT id, isFirstChoice, note, status FROM FinalGoal WHERE userId = ?", userID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var g savedGoal
		if err := rows.Scan(&g.id, &g.isFirstChoice, &g.note, &g.status); err != nil {
			t.Fatal(err)
		}
		saved = append(saved, g)
	}
	rows.Close()
	var maxID int64
	db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM FinalGoal").Scan(&maxID)
	restore := func() {
		if _, err := db.Exec("DELETE FROM FinalGoal WHERE userId = ? AND id > ?", userID, maxID); err != nil {
			t.Errorf("作った志望校を消せません: %v", err)
		}
		for _, g := range saved {
			if _, err := db.Exec("UPDATE FinalGoal SET isFirstChoice = ?, note = ?, status = ? WHERE id = ?",
				g.isFirstChoice, g.note, g.status, g.id); err != nil {
				t.Errorf("志望校 %d を戻せません: %v", g.id, err)
			}
		}
	}
	defer restore()

	// 1人目がまだ登録していない学部を3つ。ほかの人の志望校を1つ。
	var faculties []int64
	frows, err := db.Query(`SELECT id FROM Faculty
		WHERE id NOT IN (SELECT facultyId FROM FinalGoal WHERE userId = ?) ORDER BY id LIMIT 3`, userID)
	if err != nil {
		t.Fatal(err)
	}
	for frows.Next() {
		var id int64
		frows.Scan(&id)
		faculties = append(faculties, id)
	}
	frows.Close()
	if len(faculties) < 3 {
		t.Fatalf("1人目が登録していない学部が %d 件しかありません", len(faculties))
	}
	fA, fB, fC := faculties[0], faculties[1], faculties[2]
	var otherGoal int64
	if err := db.QueryRow("SELECT id FROM FinalGoal WHERE userId <> ? ORDER BY id LIMIT 1", userID).Scan(&otherGoal); err != nil {
		t.Fatal(err)
	}

	newGoalID := func(facultyID int64) int64 {
		var id int64
		if err := db.QueryRow("SELECT id FROM FinalGoal WHERE userId = ? AND facultyId = ?", userID, facultyID).Scan(&id); err != nil {
			t.Fatalf("学部 %d の志望校が見つかりません: %v", facultyID, err)
		}
		return id
	}

	run := func(base string) []response {
		defer restore()
		var out []response
		call := func(method, path, body string) {
			t.Helper()
			pr := parityRequest{method: method, path: path, cookie: cookie, body: body, header: map[string]string{}}
			if body != "" {
				pr.header["Content-Type"] = jsonType
			}
			res := send(t, base, pr)
			maskKey(res.body, "id")
			maskKey(res.body, "createdAt")
			maskEach(res.body, "id", "createdAt")
			// 500 の本文の code はそろえない。Node（error-handling.ts）は投げられたエラーの code をそのまま使い、
			// Go は INTERNAL_ERROR を返す。ステータスと文言は比べる。
			if res.status == http.StatusInternalServerError {
				maskKey(res.body, "code")
			}
			out = append(out, res)
		}
		goalPath := func(id int64) string { return fmt.Sprintf("/api/goals/%d", id) }

		// 登録。学部・大学つきで返る。同じ学部は 409、無い学部は外部キーで 500（Node と同じ）
		call("POST", "/api/goals", fmt.Sprintf(`{"facultyId":%d}`, fA))
		call("POST", "/api/goals", fmt.Sprintf(`{"facultyId":%d}`, fA))
		call("POST", "/api/goals", fmt.Sprintf(`{"facultyId":%d,"status":"candidate"}`, fB))
		call("POST", "/api/goals", `{"facultyId":999999999}`)
		g1, g2 := newGoalID(fA), newGoalID(fB)

		// 学部の差し替え。status は確かめるだけで書かない。ほかの志望校と同じ学部は一意制約で 500（Node と同じ）
		call("PUT", goalPath(g1), fmt.Sprintf(`{"facultyId":%d,"status":"candidate"}`, fC))
		call("PUT", goalPath(g1), `{}`)
		call("PUT", goalPath(g1), fmt.Sprintf(`{"facultyId":%d}`, fB))
		call("PUT", goalPath(otherGoal), fmt.Sprintf(`{"facultyId":%d}`, fC))
		call("PUT", goalPath(otherGoal), `{"facultyId":0}`)

		// 第一志望は1人1校。立てると既存の志望校の第一志望が外れる
		call("PATCH", goalPath(g1), `{"isFirstChoice":true,"note":"応答一致","status":"decided"}`)
		call("GET", "/api/goals/first-choice", "")
		call("PATCH", goalPath(g2), `{"isFirstChoice":false,"note":null}`)
		call("PATCH", goalPath(g2), `{"isFirstChoice":true}`)
		call("PATCH", goalPath(g1), `{}`)
		call("PATCH", goalPath(otherGoal), `{"note":"x"}`)
		call("GET", "/api/goals", "")

		// 削除
		call("DELETE", goalPath(g2), "")
		call("DELETE", goalPath(g2), "")
		call("DELETE", goalPath(otherGoal), "")
		call("GET", "/api/goals", "")
		return out
	}

	node := run(env.node)
	gon := run(env.goURL)
	if len(node) != len(gon) {
		t.Fatalf("段の数が違う: Node %d、Go %d", len(node), len(gon))
	}
	want := []int{201, 409, 201, 500, 200, 200, 500, 404, 400,
		200, 200, 200, 200, 200, 404, 200,
		200, 404, 404, 200}
	for i := range node {
		if i < len(want) && node[i].status != want[i] {
			t.Errorf("%d段目：Node が %d（狙いは %d）: %v", i+1, node[i].status, want[i], node[i].body)
		}
		if diffs := compareResponses(node[i], gon[i]); len(diffs) > 0 {
			t.Errorf("%d段目で食い違い（Node → Go）:\n  %s", i+1, strings.Join(diffs, "\n  "))
		}
	}
}

// TestParityTextbookScenario は参考書の成功する書き込みを比べる（JUK-75）。
//
// 同じ手順（名前で登録 → 重複 → マスターから登録 → 逆算設定の書き換え）を Node と Go で1回ずつ流し、各段の応答を比べる。
// 総量の候補が無いマスターは、手順のあいだだけ作る。
func TestParityTextbookScenario(t *testing.T) {
	env := parityEnv(t)
	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cookie := env.cookiesFor(firstUser)[0]
	userID := sessionUserID(t, db, cookie)

	var maxID int64
	db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM Textbook").Scan(&maxID)
	cleanup := func() {
		if _, err := db.Exec("DELETE FROM Textbook WHERE userId = ? AND id > ?", userID, maxID); err != nil {
			t.Errorf("作った参考書を消せません: %v", err)
		}
	}
	defer cleanup()

	// 総量の候補があり、1人目がまだ同じ名前の参考書を持っていないマスター
	var master int64
	if err := db.QueryRow(`SELECT tm.id FROM TextbookMaster AS tm
		WHERE EXISTS (SELECT 1 FROM TextbookMasterMetric AS m WHERE m.masterId = tm.id)
		  AND tm.name NOT IN (SELECT name FROM Textbook WHERE userId = ?)
		ORDER BY tm.id LIMIT 1`, userID).Scan(&master); err != nil {
		t.Fatalf("使えるマスターがありません: %v", err)
	}
	inserted, err := db.Exec(`INSERT INTO TextbookMaster (name, isbn, createdAt, updatedAt)
		VALUES ('応答一致テストの総量の無いマスター', 'parity-no-metric', UTC_TIMESTAMP(3), UTC_TIMESTAMP(3))`)
	if err != nil {
		t.Fatal(err)
	}
	noMetric, _ := inserted.LastInsertId()
	defer db.Exec("DELETE FROM TextbookMaster WHERE id = ?", noMetric)
	var otherTextbook int64
	if err := db.QueryRow("SELECT id FROM Textbook WHERE userId <> ? ORDER BY id LIMIT 1", userID).Scan(&otherTextbook); err != nil {
		t.Fatal(err)
	}
	newTextbookID := func(name string) int64 {
		var id int64
		if err := db.QueryRow("SELECT id FROM Textbook WHERE userId = ? AND name = ?", userID, name).Scan(&id); err != nil {
			t.Fatalf("参考書「%s」が見つかりません: %v", name, err)
		}
		return id
	}

	run := func(base string) []response {
		defer cleanup()
		var out []response
		call := func(method, path, body string) {
			t.Helper()
			pr := parityRequest{method: method, path: path, cookie: cookie, body: body, header: map[string]string{}}
			if body != "" {
				pr.header["Content-Type"] = jsonType
			}
			res := send(t, base, pr)
			for _, key := range []string{"id", "createdAt", "updatedAt"} {
				maskKey(res.body, key)
			}
			maskEach(res.body, "id", "createdAt", "updatedAt")
			out = append(out, res)
		}
		textbookPath := func(id int64) string { return fmt.Sprintf("/api/textbooks/%d", id) }

		// 名前で登録（前後の空白は削る）。同じ名前は 409
		call("POST", "/api/textbooks", `{"name":" 応答一致テストの参考書 ","subject":"math","rangeUnit":"page"}`)
		call("POST", "/api/textbooks", `{"name":"応答一致テストの参考書"}`)
		// 名前と masterId の両方を送ると名前で作る
		call("POST", "/api/textbooks", fmt.Sprintf(`{"name":"応答一致テストの参考書2","masterId":%d}`, master))
		// マスターから登録（総量は既定の候補）。2回目は同じ名前なので 409。候補が無い・マスターが無い
		call("POST", "/api/textbooks", fmt.Sprintf(`{"masterId":%d}`, master))
		call("POST", "/api/textbooks", fmt.Sprintf(`{"masterId":%d}`, master))
		call("POST", "/api/textbooks", fmt.Sprintf(`{"masterId":%d}`, noMetric))
		call("POST", "/api/textbooks", `{"masterId":999999999}`)
		t1 := newTextbookID("応答一致テストの参考書")

		// 逆算設定。送った項目だけが変わる（更新日時は毎回変わる）
		call("PATCH", textbookPath(t1), `{"totalAmount":300,"rangeUnit":"chapter","targetDate":"2026-12-31","subject":null}`)
		call("PATCH", textbookPath(t1), `{"targetDate":null}`)
		call("PATCH", textbookPath(t1), `{"targetDate":"2024-02-29"}`)
		call("PATCH", textbookPath(t1), `{}`)
		call("PATCH", textbookPath(otherTextbook), `{"totalAmount":1}`)
		call("PATCH", textbookPath(otherTextbook), `{"totalAmount":0}`)
		call("GET", "/api/textbooks", "")
		return out
	}

	node := run(env.node)
	gon := run(env.goURL)
	if len(node) != len(gon) {
		t.Fatalf("段の数が違う: Node %d、Go %d", len(node), len(gon))
	}
	want := []int{201, 409, 201, 201, 409, 400, 404, 200, 200, 200, 200, 404, 400, 200}
	for i := range node {
		if i < len(want) && node[i].status != want[i] {
			t.Errorf("%d段目：Node が %d（狙いは %d）: %v", i+1, node[i].status, want[i], node[i].body)
		}
		if diffs := compareResponses(node[i], gon[i]); len(diffs) > 0 {
			t.Errorf("%d段目で食い違い（Node → Go）:\n  %s", i+1, strings.Join(diffs, "\n  "))
		}
	}
}
