// Package dbtest は、本物の MySQL に流すテスト（dbtest タグ、JUK-97）の下ごしらえ。
//
// DB は db/ のテストと同じ juken_map_test を使う。本番と同じマイグレーションが当たっていて、
// 繋ぐのは本番と同じ権限（DML だけ）のユーザー。先に `pnpm --filter @juken-map/db test-db:prepare` で用意する。
//
// テストごとに使い捨てのユーザーを作り、データはすべてそのユーザーにぶら下げる（db/test-db/fixtures.ts と同じ）。
// ユーザーを消せば、志望校・予定・実績などは外部キーの CASCADE で一緒に消える。
//
// どのパッケージの DB テストからも使えるよう、普通のパッケージにしている（JUK-158）。
// テスト（dbtest タグの _test.go）からだけ import する。
package dbtest

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// DefaultURL は TEST_DATABASE_URL が無いときのテスト用 DB。db/test-db/config.ts と同じ既定値。
const DefaultURL = "mysql://juken_app_test:juken_app_test@127.0.0.1:3306/juken_map_test"

// Open はテスト用の DB に繋ぎ、テストの終わりに閉じる。名前が _test で終わらない DB には繋がない。
func Open(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = DefaultURL
	}
	// 取り違えの防止。本番や開発の DB に向いていたら、何かする前に止める。
	cfg, err := database.Config(url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.DBName, "_test") {
		t.Fatalf("テスト用 DB の名前は _test で終わる必要があります（%s）", cfg.DBName)
	}
	db, err := database.Open(url)
	if err != nil {
		t.Fatalf("テスト用の MySQL に繋げません。`pnpm db:start` と `pnpm --filter @juken-map/db test-db:prepare` を先に動かしてください: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Fixture はテストデータを作り、テストの終わりに消す。
type Fixture struct {
	T  *testing.T
	DB *sql.DB
}

func (fx Fixture) Exec(query string, args ...any) sql.Result {
	fx.T.Helper()
	res, err := fx.DB.Exec(query, args...)
	if err != nil {
		fx.T.Fatalf("%s: %v", query, err)
	}
	return res
}

func (fx Fixture) Insert(query string, args ...any) int64 {
	fx.T.Helper()
	id, err := fx.Exec(query, args...).LastInsertId()
	if err != nil {
		fx.T.Fatal(err)
	}
	return id
}

// Hex は n バイトの乱数を16進の文字列にする。名前やメールアドレスを、ほかのテストと重ならないようにする。
func Hex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (fx Fixture) User() string {
	fx.T.Helper()
	id := "test-go-" + Hex(8)
	now := time.Now()
	fx.Exec("INSERT INTO `user` (id, email, createdAt, updatedAt) VALUES (?, ?, ?, ?)", id, id+"@example.test", now, now)
	fx.T.Cleanup(func() { fx.Exec("DELETE FROM `user` WHERE id = ?", id) })
	return id
}

// University は学部を1つ持つ大学を作り、学部の ID を返す。ユーザーより先に作れば、消すのはユーザー（と志望校）の後になる
// （志望校は学部を ON DELETE RESTRICT で参照している）。
func (fx Fixture) University() int64 {
	fx.T.Helper()
	universityID := fx.Insert("INSERT INTO University (name, prefecture, type, createdAt) VALUES (?, ?, ?, ?)",
		"大学-"+Hex(8), "東京都", "私立", time.Now())
	fx.T.Cleanup(func() { fx.Exec("DELETE FROM University WHERE id = ?", universityID) })
	return fx.Insert("INSERT INTO Faculty (name, examDate, universityId, createdAt) VALUES (?, ?, ?, ?)",
		"学部", time.Date(2027, 2, 15, 0, 0, 0, 0, time.UTC), universityID, time.Now())
}

func (fx Fixture) Textbook(userID string) int64 {
	now := time.Now()
	return fx.Insert("INSERT INTO Textbook (userId, name, createdAt, updatedAt) VALUES (?, ?, ?, ?)", userID, "参考書", now, now)
}

func (fx Fixture) StudyPlan(userID string) int64 {
	now := time.Now()
	return fx.Insert(`INSERT INTO StudyPlan (userId, date, content, done, createdAt, updatedAt) VALUES (?, ?, ?, false, ?, ?)`,
		userID, time.Date(2027, 2, 20, 0, 0, 0, 0, time.UTC), "元の内容", now, now)
}

func (fx Fixture) StudyLog(userID string) int64 {
	now := time.Now()
	return fx.Insert(`INSERT INTO StudyLog (userId, date, minutes, createdAt, updatedAt) VALUES (?, ?, 30, ?, ?)`, userID, now, now, now)
}

func (fx Fixture) FinalGoal(userID string, facultyID int64) int64 {
	return fx.Insert(`INSERT INTO FinalGoal (userId, facultyId, isFirstChoice, status, createdAt) VALUES (?, ?, false, 'decided', ?)`,
		userID, facultyID, time.Now())
}

func (fx Fixture) LineConnection(userID string) {
	now := time.Now()
	fx.Exec("INSERT INTO LineConnection (userId, lineUserId, linkedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)",
		userID, "U"+Hex(16), now, now, now)
}

// Count は COUNT(*) の照会の結果を返す。
func (fx Fixture) Count(query string, args ...any) int {
	fx.T.Helper()
	var n int
	if err := fx.DB.QueryRow(query, args...).Scan(&n); err != nil {
		fx.T.Fatalf("%s: %v", query, err)
	}
	return n
}

// Rows は照会の結果を、1行ずつ JSON の文字列にして返す（列名で並ぶので比べやすい）。NULL は null になる。
// 並び順に頼らないよう、文字列で並べ替えて返す。
func (fx Fixture) Rows(query string, args ...any) []string {
	fx.T.Helper()
	res, err := fx.DB.Query(query, args...)
	if err != nil {
		fx.T.Fatalf("%s: %v", query, err)
	}
	defer res.Close()
	cols, err := res.Columns()
	if err != nil {
		fx.T.Fatal(err)
	}
	out := []string{}
	for res.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := res.Scan(ptrs...); err != nil {
			fx.T.Fatal(err)
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
		fx.T.Fatal(err)
	}
	sort.Strings(out)
	return out
}
