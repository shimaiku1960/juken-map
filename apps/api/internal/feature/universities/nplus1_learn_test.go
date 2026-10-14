//go:build learn

// 学習 1-2（JUK-183）の実験。今の listForExplore（JOIN 1本）と、わざと N+1 で書いた版を、
// 同じ DB に流して SQL の本数と時間を比べる。`-tags learn` を付けたときだけ動き、CI では動かない。
//
//	LEARN_DATABASE_URL=mysql://juken:jukenpassword@localhost:3306/juken_map \
//	  go test -tags learn -run TestLearnNPlus1 -v ./internal/feature/universities/
package universities

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// listForExploreNPlus1 は listForExplore と同じ結果を、大学ごと・学部ごとに SQL を流して作る。
// 本数は 1（大学の一覧）＋ 大学の数（学部を取る）＋ 学部の数（タグを取る）になる。
func (st *store) listForExploreNPlus1(ctx context.Context) ([]exploreUniversityDTO, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id, name, prefecture, type FROM University ORDER BY name ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	universities := make([]exploreUniversityDTO, 0)
	for rows.Next() {
		var u exploreUniversityDTO
		if err := rows.Scan(&u.ID, &u.Name, &u.Prefecture, &u.Type); err != nil {
			rows.Close()
			return nil, err
		}
		universities = append(universities, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range universities {
		facultyIDs, err := queryInt64s(ctx, st.db,
			`SELECT id FROM Faculty WHERE universityId = ? ORDER BY id ASC`, universities[i].ID)
		if err != nil {
			return nil, err
		}
		universities[i].Faculties = make([]exploreFacultyDTO, 0, len(facultyIDs))
		for _, facultyID := range facultyIDs {
			tags, err := queryTagNames(ctx, st.db, facultyID)
			if err != nil {
				return nil, err
			}
			universities[i].Faculties = append(universities[i].Faculties, exploreFacultyDTO{Tags: tags})
		}
	}
	return universities, nil
}

func queryInt64s(ctx context.Context, db *sql.DB, query string, args ...any) ([]int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func queryTagNames(ctx context.Context, db *sql.DB, facultyID int64) ([]exploreTagDTO, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT t.name FROM _FacultyToTag AS ft JOIN Tag AS t ON t.id = ft.B
		 WHERE ft.A = ? ORDER BY t.id ASC`, facultyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := make([]exploreTagDTO, 0)
	for rows.Next() {
		var tag exploreTagDTO
		if err := rows.Scan(&tag.Name); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// openLearnDB は手元の DB だけに繋ぐ。.env には本番の URL がコメントで残っているので、
// 取り違えても localhost 以外なら止める。
func openLearnDB(t testing.TB) *sql.DB {
	t.Helper()
	raw := os.Getenv("LEARN_DATABASE_URL")
	if raw == "" {
		t.Skip("LEARN_DATABASE_URL が無いので飛ばす")
	}
	return openLocalDB(t, raw)
}

// openLocalDB は localhost の DB にだけ、接続1本で繋ぐ。
func openLocalDB(t testing.TB, raw string, wraps ...func(driver.Connector) driver.Connector) *sql.DB {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if h := u.Hostname(); h != "localhost" && h != "127.0.0.1" {
		t.Fatalf("手元の DB 以外には繋がない（host=%s）", h)
	}
	db, err := database.Open(raw, wraps...)
	if err != nil {
		t.Fatal(err)
	}
	// 接続を1本にすると、SHOW SESSION STATUS の数字がこの実験の SQL だけになる
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// questions は、この接続でこれまでに MySQL が受け取った SQL の本数（Questions）を返す。
// 数える SQL 自身も1本に数えられる。
func questions(t testing.TB, db *sql.DB) int64 {
	t.Helper()
	var name string
	var n int64
	if err := db.QueryRow(`SHOW SESSION STATUS LIKE 'Questions'`).Scan(&name, &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLearnNPlus1(t *testing.T) {
	db := openLearnDB(t)
	st := newStore(db)
	ctx := context.Background()

	run := func(name string, list func(context.Context) ([]exploreUniversityDTO, error)) []exploreUniversityDTO {
		before := questions(t, db)
		start := time.Now()
		got, err := list(ctx)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		// 差から、数える SQL 自身の1本を引く
		sqls := questions(t, db) - before - 1
		t.Logf("%-6s 大学 %d 件 / SQL %4d 本 / %v", name, len(got), sqls, elapsed)
		return got
	}

	join := run("JOIN", st.listForExplore)
	nplus1 := run("N+1", st.listForExploreNPlus1)
	if !reflect.DeepEqual(join, nplus1) {
		t.Fatal("JOIN と N+1 で結果が違う")
	}
}

// BenchmarkLearnNPlus1 は同じ比べ方を何度も繰り返し、1回あたりの時間を平均で出す。
// 1回だけの計測は、DB のキャッシュが冷えているかなどで揺れるため。
func BenchmarkLearnNPlus1(b *testing.B) {
	db := openLearnDB(b)
	st := newStore(db)
	ctx := context.Background()

	for _, c := range []struct {
		name string
		list func(context.Context) ([]exploreUniversityDTO, error)
	}{
		{"JOIN", st.listForExplore},
		{"N+1", st.listForExploreNPlus1},
	} {
		b.Run(c.name, func(b *testing.B) {
			before := questions(b, db)
			for b.Loop() {
				if _, err := c.list(ctx); err != nil {
					b.Fatal(err)
				}
			}
			// 1回あたりの SQL の本数も、時間と並べて出す
			b.ReportMetric(float64(questions(b, db)-before-1)/float64(b.N), "SQL/op")
		})
	}
}

// 実験4：件数を増やしたときの伸び方を比べる。
//
// DB は全 worktree で共有しているので、開発用の juken_map には足さない。同じ MySQL に実験専用の
// juken_map_learn を作り、juken_map の大学・学部・タグを「×倍率」ぶん写して使い、終わったら消す。
// DB を作る権限が要るので root で繋ぐ（docker-compose.yml の開発用の値）。
const (
	learnAdminURL = "mysql://root:rootpassword@127.0.0.1:3306"
	learnDBName   = "juken_map_learn"
	// 写すたびに id をずらす幅。今の id（大学は 1000 未満）とぶつからなければよい。
	copyOffset = 1_000_000
)

func BenchmarkLearnNPlus1Scale(b *testing.B) {
	admin := openLocalDB(b, learnAdminURL)
	execAll(b, admin,
		"DROP DATABASE IF EXISTS "+learnDBName,
		"CREATE DATABASE "+learnDBName,
	)
	b.Cleanup(func() { execAll(b, admin, "DROP DATABASE IF EXISTS "+learnDBName) })

	// LIKE で写すと、列と索引は同じになる（外部キーは写らないが、速さには関わらない）。
	for _, table := range []string{"University", "Faculty", "_FacultyToTag", "Tag"} {
		execAll(b, admin, "CREATE TABLE "+learnDBName+"."+table+" LIKE juken_map."+table)
	}
	execAll(b, admin, "INSERT INTO "+learnDBName+".Tag SELECT * FROM juken_map.Tag")

	db := openLocalDB(b, learnAdminURL+"/"+learnDBName)
	st := newStore(db)
	ctx := context.Background()

	copies := 0
	for _, scale := range []int{1, 2, 4, 8} {
		// 足りない分だけ写し足して、juken_map の scale 倍にする
		for ; copies < scale; copies++ {
			off := copies * copyOffset
			execAll(b, admin,
				fmt.Sprintf(`INSERT INTO %s.University (id, name, prefecture, type, createdAt)
				 SELECT id + %d, CONCAT(name, '#', %d), prefecture, type, createdAt FROM juken_map.University`,
					learnDBName, off, copies),
				fmt.Sprintf(`INSERT INTO %s.Faculty (id, name, examDate, createdAt, universityId)
				 SELECT id + %d, name, examDate, createdAt, universityId + %d FROM juken_map.Faculty`,
					learnDBName, off, off),
				fmt.Sprintf(`INSERT INTO %s._FacultyToTag (A, B) SELECT A + %d, B FROM juken_map._FacultyToTag`,
					learnDBName, off),
			)
		}

		// 測る前に、2つの版の結果が同じことを確かめる
		join, err := st.listForExplore(ctx)
		if err != nil {
			b.Fatal(err)
		}
		nplus1, err := st.listForExploreNPlus1(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if !reflect.DeepEqual(join, nplus1) {
			b.Fatalf("×%d で JOIN と N+1 の結果が違う", scale)
		}

		for _, c := range []struct {
			name string
			list func(context.Context) ([]exploreUniversityDTO, error)
		}{
			{"JOIN", st.listForExplore},
			{"N+1", st.listForExploreNPlus1},
		} {
			b.Run(fmt.Sprintf("x%d_大学%d/%s", scale, len(join), c.name), func(b *testing.B) {
				before := questions(b, db)
				for b.Loop() {
					if _, err := c.list(ctx); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(questions(b, db)-before-1)/float64(b.N), "SQL/op")
			})
		}
	}
}

func execAll(t testing.TB, db *sql.DB, queries ...string) {
	t.Helper()
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// 実験5：DB までの往復を遠くしたときに、差がどう変わるかを比べる。
//
// 手元の DB は同じ Mac の中にあり、SQL 1本の往復は約0.2ms しかかからない。本番は EC2 から RDS まで
// ネットワーク越しに往復する。SQL を DB に送る直前に決めた時間だけ待ち、往復の遠さを真似る。
// InterpolateParams が有効なので、引数つきの SQL も QueryContext の1回（＝1往復）で送られる。
func BenchmarkLearnNPlus1Latency(b *testing.B) {
	raw := os.Getenv("LEARN_DATABASE_URL")
	if raw == "" {
		b.Skip("LEARN_DATABASE_URL が無いので飛ばす")
	}
	ctx := context.Background()

	for _, delay := range []time.Duration{0, 500 * time.Microsecond, time.Millisecond} {
		db := openLocalDB(b, raw, withDelay(delay))
		st := newStore(db)

		for _, c := range []struct {
			name string
			list func(context.Context) ([]exploreUniversityDTO, error)
		}{
			{"JOIN", st.listForExplore},
			{"N+1", st.listForExploreNPlus1},
		} {
			b.Run(fmt.Sprintf("遅れ%v/%s", delay, c.name), func(b *testing.B) {
				for b.Loop() {
					if _, err := c.list(ctx); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// withDelay は、SQL を DB に送る直前に delay だけ待つよう接続を包む。
// 包み方は internal/fault/inject.go と同じ（元の接続が持つ機能を減らさない）。
func withDelay(delay time.Duration) func(driver.Connector) driver.Connector {
	return func(c driver.Connector) driver.Connector {
		return &delayConnector{Connector: c, delay: delay}
	}
}

type delayConnector struct {
	driver.Connector
	delay time.Duration
}

func (c *delayConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	full, ok := conn.(delayFullConn)
	if !ok {
		return nil, fmt.Errorf("接続 %T が必要な機能を持っていない", conn)
	}
	return &delayConn{delayFullConn: full, delay: c.delay}, nil
}

type delayFullConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
	driver.SessionResetter
	driver.Validator
	driver.NamedValueChecker
}

type delayConn struct {
	delayFullConn
	delay time.Duration
}

func (c *delayConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	time.Sleep(c.delay)
	return c.delayFullConn.QueryContext(ctx, query, args)
}

func (c *delayConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	time.Sleep(c.delay)
	return c.delayFullConn.ExecContext(ctx, query, args)
}
