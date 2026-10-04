//go:build dbtest

// 本物の MySQL に流すテスト（dbtest タグ、JUK-97）の下ごしらえ。
//
// DB は Node のテストと同じ juken_map_test を使う。本番と同じマイグレーションが当たっていて、
// 繋ぐのは本番と同じ権限（DML だけ）のユーザー。先に `pnpm --filter @juken-map/api test-db:prepare` で用意する。
//
// テストごとに使い捨てのユーザーを作り、データはすべてそのユーザーにぶら下げる（Node の test-db/fixtures.ts と同じ）。
// ユーザーを消せば、志望校・予定・実績などは外部キーの CASCADE で一緒に消える。
package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// Node の test-db/config.ts と同じ既定値。
const defaultTestDatabaseURL = "mysql://juken_app_test:juken_app_test@127.0.0.1:3306/juken_map_test"

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = defaultTestDatabaseURL
	}
	// 取り違えの防止。本番や開発の DB に向いていたら、何かする前に止める。
	cfg, err := dbConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.DBName, "_test") {
		t.Fatalf("テスト用 DB の名前は _test で終わる必要があります（%s）", cfg.DBName)
	}
	db, err := openDB(url)
	if err != nil {
		t.Fatalf("テスト用の MySQL に繋げません。`pnpm db:start` と `pnpm --filter @juken-map/api test-db:prepare` を先に動かしてください: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// dbFixture はテストデータを作り、テストの終わりに消す。
type dbFixture struct {
	t  *testing.T
	db *sql.DB
}

func (fx dbFixture) exec(query string, args ...any) sql.Result {
	fx.t.Helper()
	res, err := fx.db.Exec(query, args...)
	if err != nil {
		fx.t.Fatalf("%s: %v", query, err)
	}
	return res
}

func (fx dbFixture) insert(query string, args ...any) int64 {
	fx.t.Helper()
	id, err := fx.exec(query, args...).LastInsertId()
	if err != nil {
		fx.t.Fatal(err)
	}
	return id
}

func testHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (fx dbFixture) user() string {
	fx.t.Helper()
	id := "test-go-" + testHex(8)
	now := time.Now()
	fx.exec("INSERT INTO `user` (id, email, createdAt, updatedAt) VALUES (?, ?, ?, ?)", id, id+"@example.test", now, now)
	fx.t.Cleanup(func() { fx.db.Exec("DELETE FROM `user` WHERE id = ?", id) })
	return id
}

// university は学部を1つ持つ大学を作り、学部の ID を返す。ユーザーより先に作れば、消すのはユーザー（と志望校）の後になる
// （志望校は学部を ON DELETE RESTRICT で参照している）。
func (fx dbFixture) university() int64 {
	fx.t.Helper()
	universityID := fx.insert("INSERT INTO University (name, prefecture, type, createdAt) VALUES (?, ?, ?, ?)",
		"大学-"+testHex(8), "東京都", "私立", time.Now())
	fx.t.Cleanup(func() { fx.db.Exec("DELETE FROM University WHERE id = ?", universityID) })
	return fx.insert("INSERT INTO Faculty (name, examDate, universityId, createdAt) VALUES (?, ?, ?, ?)",
		"学部", time.Date(2027, 2, 15, 0, 0, 0, 0, time.UTC), universityID, time.Now())
}

func (fx dbFixture) textbook(userID string) int64 {
	now := time.Now()
	return fx.insert("INSERT INTO Textbook (userId, name, createdAt, updatedAt) VALUES (?, ?, ?, ?)", userID, "参考書", now, now)
}

func (fx dbFixture) studyPlan(userID string) int64 {
	now := time.Now()
	return fx.insert(`INSERT INTO StudyPlan (userId, date, content, done, createdAt, updatedAt) VALUES (?, ?, ?, false, ?, ?)`,
		userID, time.Date(2027, 2, 20, 0, 0, 0, 0, time.UTC), "元の内容", now, now)
}

func (fx dbFixture) studyLog(userID string) int64 {
	now := time.Now()
	return fx.insert(`INSERT INTO StudyLog (userId, date, minutes, createdAt, updatedAt) VALUES (?, ?, 30, ?, ?)`, userID, now, now, now)
}

func (fx dbFixture) finalGoal(userID string, facultyID int64) int64 {
	return fx.insert(`INSERT INTO FinalGoal (userId, facultyId, isFirstChoice, status, createdAt) VALUES (?, ?, false, 'decided', ?)`,
		userID, facultyID, time.Now())
}

func (fx dbFixture) lineConnection(userID string) {
	now := time.Now()
	fx.exec("INSERT INTO LineConnection (userId, lineUserId, linkedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)",
		userID, "U"+testHex(16), now, now, now)
}

// rows は照会の結果を、1行ずつ JSON の文字列にして返す（列名で並ぶので比べやすい）。NULL は null になる。
// 並び順に頼らないよう、文字列で並べ替えて返す。
func (fx dbFixture) rows(query string, args ...any) []string {
	fx.t.Helper()
	res, err := fx.db.Query(query, args...)
	if err != nil {
		fx.t.Fatalf("%s: %v", query, err)
	}
	defer res.Close()
	cols, err := res.Columns()
	if err != nil {
		fx.t.Fatal(err)
	}
	out := []string{}
	for res.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := res.Scan(ptrs...); err != nil {
			fx.t.Fatal(err)
		}
		row := map[string]any{}
		for i, col := range cols {
			if values[i].Valid {
				row[col] = values[i].String
			} else {
				row[col] = nil
			}
		}
		b, _ := json.Marshal(row)
		out = append(out, string(b))
	}
	if err := res.Err(); err != nil {
		fx.t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// todayTokyo は日本時間の今日（YYYY-MM-DD）。学習記録は未来の日付を断るので、本文にはこれを使う。
func todayTokyo() string {
	return time.Now().In(tokyo).Format("2006-01-02")
}

// dbTestApp は本番と同じ registerRoutes で組んだルーター。セッションは Cookie「test」の値を利用者 ID として読む
// （Better Auth の Cookie の署名と session 表は auth_test.go が確かめるので、ここでは省く）。
type dbTestApp struct {
	rt *router
}

func newDBTestApp(db *sql.DB) dbTestApp {
	rt := newRouter(func(r *http.Request) (*session, error) {
		c, err := r.Cookie("test")
		if err != nil {
			return nil, nil
		}
		return &session{UserID: c.Value, Email: c.Value + "@example.test", Role: "user"}, nil
	})
	registerRoutes(rt, db, jobConfig{}, lineConfig{webOrigin: "https://juken-map.com"}, microcmsWebhookConfig{})
	return dbTestApp{rt: rt}
}

// userRoutes は入口が user のルート（"METHOD /path"）。skip に入れたメソッドは除く。
func (app dbTestApp) userRoutes(skip ...string) []string {
	var out []string
	for _, r := range app.rt.routes {
		if r.Access != accessUser {
			continue
		}
		method, _, _ := strings.Cut(r.Pattern, " ")
		skipped := false
		for _, s := range skip {
			skipped = skipped || method == s
		}
		if !skipped {
			out = append(out, r.Pattern)
		}
	}
	return out
}

// send は userID の利用者として叩く。body が nil でなければ JSON にして送る。
func (app dbTestApp) send(method, url string, body any, userID string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, url, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: "test", Value: userID})
	rec := httptest.NewRecorder()
	app.rt.ServeHTTP(rec, req)
	return rec
}
