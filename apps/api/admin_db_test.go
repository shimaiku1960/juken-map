//go:build dbtest

// 管理画面（利用者の管理・マスター編集、JUK-78）の Go の SQL を、本物の MySQL に流して確かめる（JUK-106）。
//
// 以前は Node と Go の応答を比べるテスト（parity タグ）がこの役目を持っていたが、Node のルートを消したとき
// （JUK-84）に一緒に消えた。ここでは期待するステータスと本文を直接書く。
//
// 管理者・2段階認証・デモの拒否はルーターの仕事で、internal/httpx/router_test.go と admin_*_test.go（偽物の store）が確かめる。
// ここでは管理者のセッションで叩き、SQL が正しい行を読み書きするか（絞り込み・件数・CASCADE・一意の重なり・
// 使われている行の拒否）だけを見る。
//
// juken_map_test は Node のテストと共有なので、ほかのテストの行があっても結果が変わらないように、
// このテストで作る行はすべて名前やメールに印（prefix）を付け、印で絞って確かめる。
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// dbAdminApp は本番と同じ registerRoutes で組んだルーター。Cookie「test」の値を、2段階認証を通した管理者の
// 利用者 ID として読む（Better Auth の Cookie と session 表の読み方は auth_test.go が確かめる）。
type dbAdminApp struct {
	t  *testing.T
	rt *httpx.Router
}

func newDBAdminApp(t *testing.T, db *sql.DB) dbAdminApp {
	rt := httpx.NewRouter(func(r *http.Request) (*httpx.Session, error) {
		c, err := r.Cookie("test")
		if err != nil {
			return nil, nil
		}
		return &httpx.Session{UserID: c.Value, Email: c.Value + "@example.test", Role: "admin", TwoFactorVerified: true}, nil
	})
	registerRoutes(rt, db, jobConfig{}, lineConfig{webOrigin: "https://juken-map.com"}, microcmsWebhookConfig{})
	return dbAdminApp{t: t, rt: rt}
}

// send は adminID の管理者として叩く。body が空でなければ JSON として送る。
func (app dbAdminApp) send(method, url, body, adminID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: "test", Value: adminID})
	rec := httptest.NewRecorder()
	app.rt.ServeHTTP(rec, req)
	return rec
}

// expect は status を確かめ、本文を out に読む（out が nil なら読まない）。
func (app dbAdminApp) expect(rec *httptest.ResponseRecorder, status int, out any) {
	app.t.Helper()
	if rec.Code != status {
		app.t.Fatalf("status = %d, want %d（%s）", rec.Code, status, rec.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			app.t.Fatalf("本文を読めない: %v（%s）", err, rec.Body.String())
		}
	}
}

// expectError は status とエラーの文言を確かめる。
func (app dbAdminApp) expectError(rec *httptest.ResponseRecorder, status int, message string) {
	app.t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	app.expect(rec, status, &body)
	if body.Error != message {
		app.t.Fatalf("error = %q, want %q", body.Error, message)
	}
}

func (fx dbFixture) count(query string, args ...any) int {
	fx.T.Helper()
	var n int
	if err := fx.DB.QueryRow(query, args...).Scan(&n); err != nil {
		fx.T.Fatalf("%s: %v", query, err)
	}
	return n
}

// adminUser は利用者を1人作る。email が空ならメールなし（NULL）。消すのはテストの終わり（ぶら下がる行は CASCADE）。
func (fx dbFixture) adminUser(email, role string, createdAt time.Time, simSeq any) string {
	fx.T.Helper()
	id := "test-go-" + dbtest.Hex(8)
	var emailValue any
	if email != "" {
		emailValue = email
	}
	fx.Exec("INSERT INTO `user` (id, email, role, simSeq, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, ?, TRUE, ?, ?)",
		id, emailValue, role, simSeq, createdAt, createdAt)
	fx.T.Cleanup(func() { fx.Exec("DELETE FROM `user` WHERE id = ?", id) })
	return id
}

func (fx dbFixture) session(userID string, createdAt time.Time) {
	_, hash := newToken()
	fx.Exec("INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
		dbtest.Hex(16), hash, userID, createdAt, createdAt.Add(time.Hour), createdAt)
}

// account はログインの手段を足す。credential はパスワード、それ以外は外部ログインの結びつき。
func (fx dbFixture) account(userID, providerID string) {
	now := time.Now()
	if providerID == "credential" {
		fx.Exec("INSERT INTO AuthPassword (userId, hash, updatedAt) VALUES (?, ?, ?)", userID, "$argon2id$test", now)
		return
	}
	fx.Exec("INSERT INTO AuthIdentity (provider, providerUserId, userId, createdAt) VALUES (?, ?, ?, ?)",
		providerID, dbtest.Hex(8), userID, now)
}

// tag はタグを1つ作る（テスト用の DB にはタグが入っていない）。
func (fx dbFixture) tag(name string) int64 {
	id := fx.Insert("INSERT INTO Tag (name, createdAt) VALUES (?, ?)", name, time.Now())
	fx.T.Cleanup(func() { fx.Exec("DELETE FROM Tag WHERE id = ?", id) })
	return id
}

// nullable は *string を、ログに出せる値にする（nil は "NULL"）。
func nullable(s *string) string {
	if s == nil {
		return "NULL"
	}
	return *s
}

// testSimSeq は sim の利用者の番号。一意なので、ほかのテストと重ならない大きさから選ぶ。
func testSimSeq() int64 {
	n, _ := rand.Int(rand.Reader, big.NewInt(500_000_000))
	return 1_500_000_000 + n.Int64()
}

func TestAdminUsersDB(t *testing.T) {
	db := dbtest.Open(t)
	fx := newDBFixture(t, db)
	app := newDBAdminApp(t, db)
	adminID := fx.adminUser("admin-"+dbtest.Hex(6)+"@example.test", "admin", time.Now(), nil)

	t.Run("一覧：種別・検索語・ページで絞り、件数と最終ログイン・記録を数える", func(t *testing.T) {
		mark := "jk106-" + dbtest.Hex(6)
		older, newer := time.Now().Add(-48*time.Hour).Truncate(time.Millisecond), time.Now().Add(-time.Hour).Truncate(time.Millisecond)
		quiet := fx.adminUser(mark+"-a@example.test", "user", older, nil)
		active := fx.adminUser(mark+"-b@example.test", "user", newer, nil)
		fx.session(active, newer.Add(time.Minute))
		fx.session(active, newer.Add(2*time.Minute))
		fx.account(active, "google")
		fx.account(active, "credential")
		fx.StudyLog(active)
		fx.StudyLog(active)
		sim := fx.adminUser(mark+"-sim@example.test", "user", newer, testSimSeq())
		seed := fx.adminUser(mark+"@synthetic.juken-map.invalid", "user", newer, nil)

		list := func(query string) apischema.AdminUserList {
			t.Helper()
			var l apischema.AdminUserList
			app.expect(app.send("GET", "/api/admin/users?"+query, "", adminID), 200, &l)
			return l
		}
		ids := func(l apischema.AdminUserList) []string {
			out := []string{}
			for _, u := range l.Users {
				out = append(out, u.ID)
			}
			return out
		}

		real := list("kind=real&q=" + mark)
		if real.Total != 2 || !slices.Equal(ids(real), []string{active, quiet}) {
			t.Fatalf("real: total=%d ids=%v, want 2 [active quiet]（新しい順）", real.Total, ids(real))
		}
		if real.Page != 1 || real.PageSize != adminUsersPageSize {
			t.Errorf("page=%d pageSize=%d", real.Page, real.PageSize)
		}
		got := real.Users[0]
		if got.Kind != apischema.UserKindReal || got.StudyLogCount != 2 || !slices.Equal(got.Providers, []string{"credential", "google"}) {
			t.Errorf("active: kind=%s studyLogCount=%d providers=%v", got.Kind, got.StudyLogCount, got.Providers)
		}
		if got.LastLoginAt == nil || got.LastStudyLogAt == nil || got.BannedAt != nil {
			t.Errorf("active: lastLoginAt=%v lastStudyLogAt=%v bannedAt=%v", got.LastLoginAt, got.LastStudyLogAt, got.BannedAt)
		}
		// 日時は ISO（UTC・ミリ秒・Z）。最終ログインは2つの session の新しいほう
		if want := dates.ISOMillis(newer.Add(2 * time.Minute)); got.LastLoginAt != nil && string(*got.LastLoginAt) != want {
			t.Errorf("lastLoginAt = %s, want %s", *got.LastLoginAt, want)
		}
		if want := dates.ISOMillis(newer); string(got.CreatedAt) != want {
			t.Errorf("createdAt = %s, want %s", got.CreatedAt, want)
		}
		if q := real.Users[1]; q.StudyLogCount != 0 || len(q.Providers) != 0 || q.LastLoginAt != nil || q.LastStudyLogAt != nil {
			t.Errorf("quiet: %+v", q)
		}

		for _, c := range []struct {
			query string
			want  []string
		}{
			{"kind=sim&q=" + mark, []string{sim}},
			{"kind=seed&q=" + mark, []string{seed}},
			{"kind=demo&q=" + mark, []string{}},
			// 前後の空白を削り、大文字と小文字は区別しない
			{"kind=real&q=%20" + strings.ToUpper(mark) + "-A%20", []string{quiet}},
			// % と _ は文字として扱う（全件一致にならない）
			{"kind=real&q=" + strings.Replace(mark, "-", "%25", 1), []string{}},
			{"kind=real&q=" + strings.Replace(mark, "-", "_", 1), []string{}},
			// 2ページ目は空。件数は変わらない
			{"kind=real&q=" + mark + "&page=2", []string{}},
		} {
			l := list(c.query)
			if !slices.Equal(ids(l), c.want) {
				t.Errorf("%s: ids=%v, want %v", c.query, ids(l), c.want)
			}
		}
		if l := list("kind=real&q=" + mark + "&page=2"); l.Total != 2 || l.Page != 2 {
			t.Errorf("page=2: total=%d page=%d", l.Total, l.Page)
		}
	})

	t.Run("概要：新しい利用者と記録が、種別ごとの数と日別の登録に入る", func(t *testing.T) {
		overview := func() apischema.AdminOverview {
			t.Helper()
			var o apischema.AdminOverview
			app.expect(app.send("GET", "/api/admin/overview", "", adminID), 200, &o)
			return o
		}
		before := overview()
		kinds := []apischema.UserKind{}
		for _, k := range before.Kinds {
			kinds = append(kinds, k.Kind)
		}
		if !slices.Equal(kinds, userKinds) {
			t.Fatalf("kinds = %v, want %v（0人の種別も同じ順に並ぶ）", kinds, userKinds)
		}

		id := fx.adminUser("jk106-"+dbtest.Hex(6)+"@example.test", "user", time.Now(), nil)
		fx.StudyLog(id)
		after := overview()
		b, a := before.Kinds[0], after.Kinds[0]
		if a.Total-b.Total != 1 || a.Verified-b.Verified != 1 || a.NewLast7Days-b.NewLast7Days != 1 ||
			a.ActiveLast7Days-b.ActiveLast7Days != 1 || a.ActiveLast30Days-b.ActiveLast30Days != 1 {
			t.Errorf("real の増え方: before=%+v after=%+v（どれも1つ増えるはず）", b, a)
		}
		today := todayTokyo()
		countOn := func(o apischema.AdminOverview) int {
			for _, d := range o.RealSignupsByDay {
				if string(d.Date) == today {
					return d.Count
				}
			}
			return 0
		}
		if countOn(after)-countOn(before) != 1 {
			t.Errorf("今日（%s）の登録: %d → %d", today, countOn(before), countOn(after))
		}
	})

	t.Run("停止→もう一度停止→解除→メール違い→削除", func(t *testing.T) {
		facultyID := fx.University()
		email := "jk106-" + dbtest.Hex(6) + "@example.test"
		id := fx.adminUser(email, "user", time.Now(), nil)
		fx.session(id, time.Now())
		fx.session(id, time.Now())
		fx.StudyLog(id)
		fx.StudyLog(id)
		fx.StudyPlan(id)
		fx.Textbook(id)
		fx.FinalGoal(id, facultyID)
		bannedAt := func() *string {
			t.Helper()
			var v sql.NullString
			if err := db.QueryRow("SELECT bannedAt FROM `user` WHERE id = ?", id).Scan(&v); err != nil {
				t.Fatal(err)
			}
			if !v.Valid {
				return nil
			}
			return &v.String
		}

		var ban apischema.AdminBanResult
		app.expect(app.send("POST", "/api/admin/users/"+id+"/ban", "", adminID), 200, &ban)
		if ban.ID != id || ban.Email == nil || *ban.Email != email || ban.SessionsRemoved != 2 {
			t.Fatalf("停止の応答: %+v", ban)
		}
		first := bannedAt()
		if first == nil || fx.count("SELECT COUNT(*) FROM AuthSession WHERE userId = ?", id) != 0 {
			t.Fatalf("停止の印か session の削除が DB に無い: bannedAt=%v", first)
		}

		// 押し直しても最初に止めた日時のまま。消す session はもう無い
		var again apischema.AdminBanResult
		app.expect(app.send("POST", "/api/admin/users/"+id+"/ban", "", adminID), 200, &again)
		if again.SessionsRemoved != 0 || again.BannedAt != ban.BannedAt {
			t.Errorf("押し直しの応答: %+v（最初は %s）", again, ban.BannedAt)
		}
		if second := bannedAt(); second == nil || *second != *first {
			t.Errorf("押し直しで bannedAt が変わった: %s → %v", *first, nullable(second))
		}

		var unban apischema.AdminUserRef
		app.expect(app.send("POST", "/api/admin/users/"+id+"/unban", "", adminID), 200, &unban)
		if unban.ID != id || bannedAt() != nil {
			t.Fatalf("解除: 応答 %+v、bannedAt=%v", unban, bannedAt())
		}

		app.expectError(app.send("DELETE", "/api/admin/users/"+id, `{"email":"someone-else@example.test"}`, adminID),
			400, "メールアドレスが一致しません")
		if fx.count("SELECT COUNT(*) FROM `user` WHERE id = ?", id) != 1 {
			t.Fatal("メールが違うのに消えた")
		}

		// 大文字と前後の空白は無視して比べる
		var removed apischema.AdminDeleteResult
		app.expect(app.send("DELETE", "/api/admin/users/"+id, `{"email":"  `+strings.ToUpper(email)+`  "}`, adminID), 200, &removed)
		if removed.ID != id || removed.Removed.StudyLogs != 2 || removed.Removed.StudyPlans != 1 ||
			removed.Removed.Textbooks != 1 || removed.Removed.FinalGoals != 1 {
			t.Errorf("削除の応答: %+v", removed)
		}
		for _, table := range []string{"`user` WHERE id", "StudyLog WHERE userId", "StudyPlan WHERE userId", "Textbook WHERE userId", "FinalGoal WHERE userId"} {
			if n := fx.count("SELECT COUNT(*) FROM "+table+" = ?", id); n != 0 {
				t.Errorf("%s: %d 行残っている（CASCADE で消えるはず）", table, n)
			}
		}
		app.expectError(app.send("POST", "/api/admin/users/"+id+"/ban", "", adminID), 404, "ユーザーが見つかりません")
	})

	t.Run("守られている相手は、DB の role・email を読んで断る", func(t *testing.T) {
		otherAdmin := fx.adminUser("jk106-"+dbtest.Hex(6)+"@example.test", "admin", time.Now(), nil)
		noEmail := fx.adminUser("", "user", time.Now(), nil)
		for _, c := range []struct {
			name, method, path, body string
			status                   int
			message                  string
		}{
			{"自分自身の停止", "POST", "/api/admin/users/" + adminID + "/ban", "", 409, protectedMessages[protectedSelf]},
			{"他の管理者の停止", "POST", "/api/admin/users/" + otherAdmin + "/ban", "", 409, protectedMessages[protectedAdmin]},
			{"他の管理者の削除", "DELETE", "/api/admin/users/" + otherAdmin, `{"email":"x"}`, 409, protectedMessages[protectedAdmin]},
			{"メールの無い利用者の削除", "DELETE", "/api/admin/users/" + noEmail, `{"email":" "}`, 409, protectedMessages[protectedNoEmail]},
			{"いない相手の解除", "POST", "/api/admin/users/no-such-user/unban", "", 404, "ユーザーが見つかりません"},
		} {
			t.Run(c.name, func(t *testing.T) {
				app.expectError(app.send(c.method, c.path, c.body, adminID), c.status, c.message)
			})
		}
		if fx.count("SELECT COUNT(*) FROM `user` WHERE id IN (?, ?, ?) AND bannedAt IS NULL", adminID, otherAdmin, noEmail) != 3 {
			t.Error("断ったはずの相手が止まったか消えた")
		}
	})
}

func TestAdminMastersDB(t *testing.T) {
	db := dbtest.Open(t)
	fx := newDBFixture(t, db)
	app := newDBAdminApp(t, db)
	adminID := fx.adminUser("admin-"+dbtest.Hex(6)+"@example.test", "admin", time.Now(), nil)

	// このテストで作る大学・参考書の名前は、この印で始める（最後にまとめて消す。途中で落ちても残さない）。
	mark := "jk106-" + dbtest.Hex(6)
	t.Cleanup(func() {
		fx.Exec(`DELETE g FROM FinalGoal g JOIN Faculty f ON f.id = g.facultyId JOIN University u ON u.id = f.universityId
		         WHERE u.name LIKE ?`, mark+"%")
		fx.Exec("DELETE FROM University WHERE name LIKE ?", mark+"%")
		fx.Exec("DELETE t FROM Textbook t JOIN TextbookMaster tm ON tm.id = t.masterId WHERE tm.name LIKE ?", mark+"%")
		fx.Exec("DELETE FROM TextbookMaster WHERE name LIKE ?", mark+"%")
	})
	tag1, tag2 := fx.tag(mark+"-tag1"), fx.tag(mark+"-tag2")

	t.Run("大学と学部：作る→重なり→書き換え→学部→使われていると消せない→消す", func(t *testing.T) {
		nameA, nameB := mark+"-a", mark+"-b"
		// 大学を探す画面の一覧（GET /api/universities）に出ているか。キャッシュを捨てられているかを見る。
		listed := func(name string) bool {
			t.Helper()
			var list []struct {
				Name string `json:"name"`
			}
			app.expect(app.send("GET", "/api/universities", "", adminID), 200, &list)
			return slices.ContainsFunc(list, func(u struct {
				Name string `json:"name"`
			}) bool {
				return u.Name == name
			})
		}
		if listed(nameA) {
			t.Fatal("作る前から一覧にある")
		}

		var a, b apischema.AdminUniversity
		app.expect(app.send("POST", "/api/admin/universities", `{"name":" `+nameA+` ","prefecture":"北海道","type":"私立"}`, adminID), 201, &a)
		if a.Name != nameA || a.Prefecture != "北海道" || a.Type != "私立" || a.FacultyCount != 0 || a.GoalCount != 0 {
			t.Fatalf("作った大学: %+v", a)
		}
		app.expect(app.send("POST", "/api/admin/universities", `{"name":"`+nameB+`","prefecture":"東京都","type":"国立"}`, adminID), 201, &b)
		if !listed(nameA) {
			t.Fatal("作った大学が、大学を探す画面の一覧にすぐ出ない（キャッシュを捨てていない）")
		}
		app.expectError(app.send("POST", "/api/admin/universities", `{"name":"`+nameA+`","prefecture":"北海道","type":"私立"}`, adminID),
			409, universityMessages.duplicate)

		var updated apischema.AdminUniversity
		app.expect(app.send("PATCH", fmt.Sprintf("/api/admin/universities/%d", a.ID), `{"name":"`+nameA+`","prefecture":"東京都","type":"国立"}`, adminID), 200, &updated)
		if updated.ID != a.ID || updated.Prefecture != "東京都" || updated.Type != "国立" {
			t.Errorf("書き換えた大学: %+v", updated)
		}
		// ほかの大学の名前に書き換えようとすると重なる
		app.expectError(app.send("PATCH", fmt.Sprintf("/api/admin/universities/%d", a.ID), `{"name":"`+nameB+`","prefecture":"東京都","type":"国立"}`, adminID),
			409, universityMessages.duplicate)
		app.expectError(app.send("PATCH", "/api/admin/universities/999999999", `{"name":"`+mark+`-x","prefecture":"東京都","type":"国立"}`, adminID),
			404, universityMessages.notFound)

		var list apischema.AdminUniversityList
		app.expect(app.send("GET", "/api/admin/universities?q="+mark, "", adminID), 200, &list)
		if list.Total != 2 || len(list.Universities) != 2 || list.Universities[0].ID != a.ID || list.Universities[1].ID != b.ID {
			t.Errorf("検索: total=%d universities=%+v（名前の順）", list.Total, list.Universities)
		}

		// 学部：タグは送った順に関わらず id の順で返る
		var faculty apischema.AdminFacultySnapshot
		facultyBody := fmt.Sprintf(`{"name":" 法学部 ","examDate":"2027-02-15","tagIds":[%d,%d],"universityId":%d}`, tag2, tag1, a.ID)
		app.expect(app.send("POST", "/api/admin/faculties", facultyBody, adminID), 201, &faculty)
		if faculty.Name != "法学部" || faculty.ExamDate != "2027-02-15" || faculty.UniversityID != a.ID || !slices.Equal(faculty.TagIds, []int64{tag1, tag2}) {
			t.Fatalf("作った学部: %+v", faculty)
		}
		app.expectError(app.send("POST", "/api/admin/faculties", facultyBody, adminID), 409, facultyMessages.duplicate)
		// 同じ名前でも別の大学なら作れる
		var other apischema.AdminFacultySnapshot
		app.expect(app.send("POST", "/api/admin/faculties", fmt.Sprintf(`{"name":"法学部","examDate":"2027-02-15","tagIds":[],"universityId":%d}`, b.ID), adminID), 201, &other)
		app.expectError(app.send("POST", "/api/admin/faculties", `{"name":"法学部","examDate":"2027-02-15","tagIds":[],"universityId":999999999}`, adminID),
			404, facultyMessages.notFound)
		app.expectError(app.send("POST", "/api/admin/faculties", fmt.Sprintf(`{"name":"経済学部","examDate":"2027-02-15","tagIds":[%d,999999999],"universityId":%d}`, tag1, a.ID), adminID),
			400, "存在しないタグが含まれています")
		if n := fx.count("SELECT COUNT(*) FROM Faculty WHERE universityId = ?", a.ID); n != 1 {
			t.Fatalf("断ったはずの学部が増えた: %d", n)
		}

		var changed apischema.AdminFacultySnapshot
		app.expect(app.send("PATCH", fmt.Sprintf("/api/admin/faculties/%d", faculty.ID), fmt.Sprintf(`{"name":"経済学部","examDate":"2028-01-15","tagIds":[%d]}`, tag2), adminID), 200, &changed)
		if changed.Name != "経済学部" || changed.ExamDate != "2028-01-15" || !slices.Equal(changed.TagIds, []int64{tag2}) {
			t.Errorf("書き換えた学部: %+v", changed)
		}
		if n := fx.count("SELECT COUNT(*) FROM _FacultyToTag WHERE A = ?", faculty.ID); n != 1 {
			t.Errorf("タグの付け替えで中間テーブルが %d 行（1行のはず）", n)
		}

		var detail apischema.AdminUniversityDetail
		app.expect(app.send("GET", fmt.Sprintf("/api/admin/universities/%d", a.ID), "", adminID), 200, &detail)
		if detail.University.FacultyCount != 1 || len(detail.Faculties) != 1 {
			t.Fatalf("詳細: %+v", detail)
		}
		if f := detail.Faculties[0]; f.Name != "経済学部" || f.ExamDate != "2028-01-15" || len(f.Tags) != 1 || f.Tags[0].ID != tag2 || f.Tags[0].Name != mark+"-tag2" {
			t.Errorf("詳細の学部: %+v", f)
		}

		// 志望校に使われている学部・大学は消せない
		user := fx.User()
		fx.FinalGoal(user, faculty.ID)
		app.expectError(app.send("DELETE", fmt.Sprintf("/api/admin/faculties/%d", faculty.ID), "", adminID), 409, fmt.Sprintf(facultyMessages.inUse, 1))
		app.expectError(app.send("DELETE", fmt.Sprintf("/api/admin/universities/%d", a.ID), "", adminID), 409, fmt.Sprintf(universityMessages.inUse, 1))
		fx.Exec("DELETE FROM FinalGoal WHERE userId = ?", user)

		app.expect(app.send("DELETE", fmt.Sprintf("/api/admin/faculties/%d", faculty.ID), "", adminID), 204, nil)
		if n := fx.count("SELECT COUNT(*) FROM _FacultyToTag WHERE A = ?", faculty.ID); n != 0 {
			t.Errorf("学部のタグが CASCADE で消えていない: %d", n)
		}
		app.expectError(app.send("DELETE", fmt.Sprintf("/api/admin/faculties/%d", faculty.ID), "", adminID), 404, facultyMessages.notFound)
		// 学部ごと消える（CASCADE）
		app.expect(app.send("DELETE", fmt.Sprintf("/api/admin/universities/%d", b.ID), "", adminID), 204, nil)
		if n := fx.count("SELECT COUNT(*) FROM Faculty WHERE id = ?", other.ID); n != 0 {
			t.Errorf("大学を消しても学部が残っている")
		}
		app.expect(app.send("DELETE", fmt.Sprintf("/api/admin/universities/%d", a.ID), "", adminID), 204, nil)
		app.expectError(app.send("GET", fmt.Sprintf("/api/admin/universities/%d", a.ID), "", adminID), 404, universityMessages.notFound)
		if listed(nameA) {
			t.Fatal("消した大学が、大学を探す画面の一覧に残っている（キャッシュを捨てていない）")
		}
	})

	t.Run("参考書：作る→重なり→書き換え→検索→使われていると消せない→消す", func(t *testing.T) {
		// ISBN は13桁の数字。ハイフンは API が取り除く
		digits := fmt.Sprintf("%08d", time.Now().UnixNano()%1e8)
		isbn, isbn2 := "979-1"+digits+"-1", "979-1"+digits+"-2"
		name := mark + "-book"
		body := func(name, isbn string) string {
			return fmt.Sprintf(`{"name":%q,"publisher":" 社 ","edition":"","isbn":%q,"metrics":[{"unit":"page","totalAmount":300,"isDefault":true},{"unit":"chapter","totalAmount":12,"isDefault":false}]}`, name, isbn)
		}

		// 利用者が選ぶ一覧（GET /api/textbook-masters）での見え方。キャッシュを捨てられているかを見る。
		listed := func(id int64) *apischema.TextbookMaster {
			t.Helper()
			var list []apischema.TextbookMaster
			app.expect(app.send("GET", "/api/textbook-masters", "", adminID), 200, &list)
			if i := slices.IndexFunc(list, func(m apischema.TextbookMaster) bool { return m.ID == id }); i >= 0 {
				return &list[i]
			}
			return nil
		}
		listed(0) // 作る前に一覧をキャッシュに載せておく

		var created apischema.AdminTextbookMaster
		app.expect(app.send("POST", "/api/admin/textbook-masters", body(name, isbn), adminID), 201, &created)
		if listed(created.ID) == nil {
			t.Error("作った参考書が利用者の一覧に出ない（キャッシュが残っている）")
		}
		if created.Name != name || created.Isbn != strings.ReplaceAll(isbn, "-", "") || created.Publisher == nil || *created.Publisher != "社" ||
			created.Edition != nil || created.TextbookCount != 0 {
			t.Fatalf("作った参考書: %+v", created)
		}
		if want := []apischema.AdminTextbookMasterMetric{{Unit: "page", TotalAmount: 300, IsDefault: true}, {Unit: "chapter", TotalAmount: 12}}; !slices.Equal(created.Metrics, want) {
			t.Errorf("総量の候補: %+v, want %+v", created.Metrics, want)
		}
		app.expectError(app.send("POST", "/api/admin/textbook-masters", body(mark+"-dup", isbn), adminID), 409, textbookMasterMessages.duplicate)
		if n := fx.count("SELECT COUNT(*) FROM TextbookMaster WHERE name = ?", mark+"-dup"); n != 0 {
			t.Errorf("重なりで断ったのに行がある（トランザクションが戻っていない）")
		}

		var other apischema.AdminTextbookMaster
		app.expect(app.send("POST", "/api/admin/textbook-masters", body(mark+"-other", isbn2), adminID), 201, &other)
		// ほかの参考書の ISBN に書き換えようとすると重なり、総量の候補も元のまま
		app.expectError(app.send("PATCH", fmt.Sprintf("/api/admin/textbook-masters/%d", created.ID), body(name, isbn2), adminID),
			409, textbookMasterMessages.duplicate)
		if n := fx.count("SELECT COUNT(*) FROM TextbookMasterMetric WHERE masterId = ?", created.ID); n != 2 {
			t.Errorf("断った書き換えで総量の候補が %d 行（2行のはず）", n)
		}

		var updated apischema.AdminTextbookMaster
		app.expect(app.send("PATCH", fmt.Sprintf("/api/admin/textbook-masters/%d", created.ID),
			fmt.Sprintf(`{"name":%q,"publisher":null,"edition":" 第3版 ","isbn":%q,"metrics":[{"unit":"question","totalAmount":450,"isDefault":true}]}`, name, isbn), adminID), 200, &updated)
		if updated.Publisher != nil || updated.Edition == nil || *updated.Edition != "第3版" ||
			!slices.Equal(updated.Metrics, []apischema.AdminTextbookMasterMetric{{Unit: "question", TotalAmount: 450, IsDefault: true}}) {
			t.Errorf("書き換えた参考書: %+v", updated)
		}
		if m := listed(created.ID); m == nil || m.Edition == nil || *m.Edition != "第3版" || len(m.Metrics) != 1 {
			t.Errorf("書き換えが利用者の一覧に出ない: %+v", m)
		}
		app.expectError(app.send("PATCH", "/api/admin/textbook-masters/999999999", body(name, isbn), adminID), 404, textbookMasterMessages.notFound)

		// 名前でも ISBN でも探せる
		for _, q := range []string{name, strings.ReplaceAll(isbn, "-", "")} {
			var found []apischema.AdminTextbookMaster
			app.expect(app.send("GET", "/api/admin/textbook-masters?q="+q, "", adminID), 200, &found)
			if len(found) != 1 || found[0].ID != created.ID {
				t.Errorf("q=%s: %+v", q, found)
			}
		}

		// 利用者の参考書が使っていれば消せない
		user := fx.User()
		fx.Exec("INSERT INTO Textbook (userId, name, masterId, updatedAt) VALUES (?, ?, ?, ?)", user, name, created.ID, time.Now())
		var used []apischema.AdminTextbookMaster
		app.expect(app.send("GET", "/api/admin/textbook-masters?q="+name, "", adminID), 200, &used)
		if len(used) != 1 || used[0].TextbookCount != 1 {
			t.Errorf("使われている数: %+v", used)
		}
		app.expectError(app.send("DELETE", fmt.Sprintf("/api/admin/textbook-masters/%d", created.ID), "", adminID), 409, fmt.Sprintf(textbookMasterMessages.inUse, 1))
		fx.Exec("DELETE FROM Textbook WHERE userId = ?", user)

		app.expect(app.send("DELETE", fmt.Sprintf("/api/admin/textbook-masters/%d", created.ID), "", adminID), 204, nil)
		if listed(created.ID) != nil {
			t.Error("消した参考書が利用者の一覧に残っている")
		}
		app.expectError(app.send("DELETE", fmt.Sprintf("/api/admin/textbook-masters/%d", created.ID), "", adminID), 404, textbookMasterMessages.notFound)
		if n := fx.count("SELECT COUNT(*) FROM TextbookMasterMetric WHERE masterId = ?", created.ID); n != 0 {
			t.Errorf("総量の候補が CASCADE で消えていない: %d", n)
		}
	})

	t.Run("特殊文字を含む名前は、SQL として読まれず文字どおり保存・検索される", func(t *testing.T) {
		// セキュリティ基準 D1（インジェクション）。値を ? で渡していれば、引用符・バックスラッシュ・コメントの記号・
		// LIKE の % と _ を含んでも、そのままの文字列として扱われる。連結で組み立てていれば、構文エラー（500）か
		// 別の SQL になる。
		name := mark + `-x'); DROP TABLE University; -- \' " %_`
		var created apischema.AdminUniversity
		body, _ := json.Marshal(map[string]string{"name": name, "prefecture": "東京都", "type": "私立"})
		app.expect(app.send("POST", "/api/admin/universities", string(body), adminID), 201, &created)
		if created.Name != name {
			t.Fatalf("name = %q, want %q", created.Name, name)
		}
		var stored string
		if err := db.QueryRow("SELECT name FROM University WHERE id = ?", created.ID).Scan(&stored); err != nil || stored != name {
			t.Fatalf("DB の name = %q（%v）, want %q", stored, err, name)
		}

		// 検索語にも同じ文字を入れる。LIKE の % と _ も文字として扱うので、ちょうどこの大学だけに当たる
		var list apischema.AdminUniversityList
		app.expect(app.send("GET", "/api/admin/universities?q="+url.QueryEscape(`'); DROP TABLE University; -- \' " %_`), "", adminID), 200, &list)
		if list.Total != 1 || len(list.Universities) != 1 || list.Universities[0].ID != created.ID {
			t.Errorf("検索: total=%d universities=%+v", list.Total, list.Universities)
		}
		if n := fx.count("SELECT COUNT(*) FROM University WHERE id = ?", created.ID); n != 1 {
			t.Errorf("University の行が %d 件（表ごと消えていないか）", n)
		}
		app.expect(app.send("DELETE", fmt.Sprintf("/api/admin/universities/%d", created.ID), "", adminID), 204, nil)
	})

	t.Run("タグの一覧", func(t *testing.T) {
		var tags []apischema.AdminTag
		app.expect(app.send("GET", "/api/admin/tags", "", adminID), 200, &tags)
		i := slices.IndexFunc(tags, func(tag apischema.AdminTag) bool { return tag.ID == tag1 })
		if i < 0 || i+1 >= len(tags) || tags[i].Name != mark+"-tag1" || tags[i+1].ID != tag2 {
			t.Errorf("作ったタグが id の順に並んでいない: %+v", tags)
		}
	})
}
