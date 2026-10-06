//go:build dbtest

// マイグレーションの適用（migrate.go）を、本物の MySQL に作る使い捨ての DB で確かめる（JUK-125）。
// Node の apps/api/src/infra/migrations.test.ts から移した。
//
// テスト用 DB（juken_map_test）は他のテストが使っているので、別の DB（juken_map_migrate_test）を毎回作り直す。
// 当てるのは本番と同じ migrate の権限（db/db-users.ts）のユーザー juken_migrations_test で、
// その権限で db/migrations を全部当てられることも、ここで確かめる。ユーザーは test-db:prepare が作る。
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

const (
	migrateTestDBName = "juken_map_migrate_test"
	// go test はパッケージのディレクトリ（apps/api）で動く。
	repoMigrationsDir = "../../../../db/migrations"
)

type migrateFixture struct {
	t     *testing.T
	admin *sql.DB
	url   string
}

func newMigrateFixture(t *testing.T) migrateFixture {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_ADMIN_URL")
	if adminURL == "" {
		adminURL = "mysql://root:rootpassword@127.0.0.1:3306"
	}
	admin, err := database.Open(adminURL)
	if err != nil {
		t.Fatalf("テスト用の MySQL に繋げません: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS `" + migrateTestDBName + "`"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	for _, q := range []string{
		"DROP DATABASE IF EXISTS `" + migrateTestDBName + "`",
		"CREATE DATABASE `" + migrateTestDBName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
	} {
		if _, err := admin.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	// 接続先は TEST_DATABASE_URL と同じ MySQL の、別の DB・別のユーザー（db/test-db/config.ts と同じ作り方）。
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		base = dbtest.DefaultURL
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("juken_migrations_test", "juken_migrations_test")
	u.Path = "/" + migrateTestDBName
	return migrateFixture{t: t, admin: admin, url: u.String()}
}

func (fx migrateFixture) apply(dir string) ([]string, error) {
	return Apply(context.Background(), fx.url, dir, io.Discard)
}

func (fx migrateFixture) mustApply(dir string) []string {
	fx.t.Helper()
	names, err := fx.apply(dir)
	if err != nil {
		fx.t.Fatal(err)
	}
	return names
}

type ledgerRow struct {
	name       string
	checksum   string
	finished   bool
	rolledBack bool
	logs       string
}

func (fx migrateFixture) ledger() []ledgerRow {
	fx.t.Helper()
	rows, err := fx.admin.Query("SELECT migration_name, checksum, finished_at IS NOT NULL, rolled_back_at IS NOT NULL, COALESCE(logs, '') FROM `" +
		migrateTestDBName + "`._prisma_migrations ORDER BY started_at, migration_name")
	if err != nil {
		fx.t.Fatal(err)
	}
	defer rows.Close()
	var out []ledgerRow
	for rows.Next() {
		var r ledgerRow
		if err := rows.Scan(&r.name, &r.checksum, &r.finished, &r.rolledBack, &r.logs); err != nil {
			fx.t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func (fx migrateFixture) tables() []string {
	fx.t.Helper()
	rows, err := fx.admin.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name", migrateTestDBName)
	if err != nil {
		fx.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			fx.t.Fatal(err)
		}
		out = append(out, name)
	}
	return out
}

// migrationsDir は一時ディレクトリに migrations を作る。{ 名前: SQL }
func migrationsDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeMigration(t, dir, name, content)
	}
	return dir
}

func writeMigration(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "migration.sql"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateDB(t *testing.T) {
	t.Run("空の DB に db/migrations を名前順にすべて当て、2回目は何も当てない", func(t *testing.T) {
		fx := newMigrateFixture(t)
		names, err := migrationNames(repoMigrationsDir)
		if err != nil {
			t.Fatal(err)
		}
		if got := fx.mustApply(repoMigrationsDir); !reflect.DeepEqual(got, names) {
			t.Fatalf("当てたもの = %v, want %v", got, names)
		}
		if got := fx.mustApply(repoMigrationsDir); len(got) != 0 {
			t.Fatalf("2回目に当てたもの = %v, want なし", got)
		}

		// 記録は Prisma と同じ形式（checksum は migration.sql の SHA-256）
		records := fx.ledger()
		if len(records) != len(names) {
			t.Fatalf("記録 %d 件, want %d", len(records), len(names))
		}
		for i, r := range records {
			content, err := os.ReadFile(filepath.Join(repoMigrationsDir, r.name, "migration.sql"))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(content)
			if r.name != names[i] || r.checksum != hex.EncodeToString(sum[:]) || !r.finished {
				t.Errorf("記録 %+v が %s のものと合わない", r, names[i])
			}
		}
		tables := fx.tables()
		for _, want := range []string{"user", "StudyPlan", "StudyLog"} {
			if !slices.Contains(tables, want) {
				t.Errorf("表 %s ができていない: %v", want, tables)
			}
		}
	})

	t.Run("当て済みの記録があるものは飛ばし、足されたものだけを当てる", func(t *testing.T) {
		fx := newMigrateFixture(t)
		dir := migrationsDir(t, map[string]string{
			"001_a": "CREATE TABLE a (id int PRIMARY KEY);",
			"002_b": "CREATE TABLE b (id int PRIMARY KEY);\nCREATE TABLE b2 (id int PRIMARY KEY);",
		})
		if got := fx.mustApply(dir); !reflect.DeepEqual(got, []string{"001_a", "002_b"}) {
			t.Fatalf("1回目 = %v", got)
		}
		writeMigration(t, dir, "003_c", "ALTER TABLE a ADD COLUMN note text;")
		if got := fx.mustApply(dir); !reflect.DeepEqual(got, []string{"003_c"}) {
			t.Fatalf("2回目 = %v", got)
		}
		if got := fx.tables(); !reflect.DeepEqual(got, []string{"_prisma_migrations", "a", "b", "b2"}) {
			t.Fatalf("表 = %v", got)
		}
	})

	t.Run("失敗したら記録を残して止まり、直すまで後ろのものも当てない", func(t *testing.T) {
		fx := newMigrateFixture(t)
		dir := migrationsDir(t, map[string]string{
			"001_ok":     "CREATE TABLE a (id int PRIMARY KEY);",
			"002_broken": "CREATE TABLE b (id int PRIMARY KEY);\nALTER TABLE no_such_table ADD COLUMN x int;",
			"003_later":  "CREATE TABLE c (id int PRIMARY KEY);",
		})
		// 2つ目の文の失敗も拾えること（複数文のうち最初の文だけ見て成功扱いにしない）。
		if _, err := fx.apply(dir); err == nil || !strings.Contains(err.Error(), "002_broken の適用に失敗しました") {
			t.Fatalf("err = %v", err)
		}
		records := fx.ledger()
		if len(records) != 2 || records[0].name != "001_ok" || !records[0].finished ||
			records[1].name != "002_broken" || records[1].finished {
			t.Fatalf("記録 = %+v", records)
		}
		if !strings.Contains(records[1].logs, "no_such_table") {
			t.Errorf("logs = %q", records[1].logs)
		}
		// 途中まで当たった（b はできた）状態のまま、次の実行も止まる
		if _, err := fx.apply(dir); err == nil || !strings.Contains(err.Error(), "途中で失敗したマイグレーションがあります: 002_broken") {
			t.Fatalf("2回目の err = %v", err)
		}
		if slices.Contains(fx.tables(), "c") {
			t.Error("後ろの 003_later が当たっている")
		}
	})

	t.Run("取り消した記録（rolled_back_at）のものは、直したあとでもう一度当てられる", func(t *testing.T) {
		fx := newMigrateFixture(t)
		dir := migrationsDir(t, map[string]string{"001_broken": "ALTER TABLE no_such_table ADD COLUMN x int;"})
		if _, err := fx.apply(dir); err == nil {
			t.Fatal("失敗するはず")
		}
		writeMigration(t, dir, "001_broken", "CREATE TABLE fixed (id int PRIMARY KEY);")
		if _, err := fx.admin.Exec("UPDATE `" + migrateTestDBName + "`._prisma_migrations SET rolled_back_at = NOW(3) WHERE migration_name = '001_broken'"); err != nil {
			t.Fatal(err)
		}
		if got := fx.mustApply(dir); !reflect.DeepEqual(got, []string{"001_broken"}) {
			t.Fatalf("当てたもの = %v", got)
		}
		records := fx.ledger()
		if len(records) != 2 || records[0].finished || !records[0].rolledBack || !records[1].finished || records[1].rolledBack {
			t.Fatalf("記録 = %+v", records)
		}
	})

	t.Run("同時に2つ走っても、当てるのは1回だけ", func(t *testing.T) {
		fx := newMigrateFixture(t)
		dir := migrationsDir(t, map[string]string{
			"001_a": "CREATE TABLE a (id int PRIMARY KEY);",
			"002_b": "CREATE TABLE b (id int PRIMARY KEY);",
		})
		var wg sync.WaitGroup
		results := make([][]string, 2)
		errs := make([]error, 2)
		for i := range 2 {
			wg.Go(func() { results[i], errs[i] = fx.apply(dir) })
		}
		wg.Wait()
		if err := errors.Join(errs...); err != nil {
			t.Fatal(err)
		}
		all := slices.Concat(results[0], results[1])
		slices.Sort(all)
		if !reflect.DeepEqual(all, []string{"001_a", "002_b"}) {
			t.Fatalf("当てたもの = %v", all)
		}
		if n := len(fx.ledger()); n != 2 {
			t.Fatalf("記録 %d 件, want 2", n)
		}
	})

	t.Run("当てたあとで migration.sql を書き換えたら警告する（流し直しはしない）", func(t *testing.T) {
		fx := newMigrateFixture(t)
		dir := migrationsDir(t, map[string]string{"001_a": "CREATE TABLE a (id int PRIMARY KEY);"})
		fx.mustApply(dir)
		writeMigration(t, dir, "001_a", "CREATE TABLE a (id bigint PRIMARY KEY);")
		var log strings.Builder
		got, err := Apply(context.Background(), fx.url, dir, &log)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
		if !strings.Contains(log.String(), "001_a は当てたあとで") {
			t.Fatalf("log = %q", log.String())
		}
	})

	t.Run("マイグレーションが1本も無いディレクトリでは、DB に触らず止まる", func(t *testing.T) {
		fx := newMigrateFixture(t)
		if _, err := fx.apply(t.TempDir()); err == nil || !strings.Contains(err.Error(), "マイグレーションがありません") {
			t.Fatalf("err = %v", err)
		}
		if got := fx.tables(); len(got) != 0 {
			t.Fatalf("表 = %v", got)
		}
	})

	// Node の dbUsers.test.ts から移した。デプロイでは migrate のユーザーで当てる。
	t.Run("アプリのユーザー（DML だけ）では当てられない", func(t *testing.T) {
		appURL := os.Getenv("TEST_DATABASE_URL")
		if appURL == "" {
			appURL = dbtest.DefaultURL
		}
		_, err := Apply(context.Background(), appURL, repoMigrationsDir, io.Discard)
		var myErr *mysql.MySQLError
		// 1142 は ER_TABLEACCESS_DENIED_ERROR（_prisma_migrations の CREATE が拒まれる）
		if !errors.As(err, &myErr) || myErr.Number != 1142 {
			t.Fatalf("err = %v, want 権限が無いことによる拒否（1142）", err)
		}
	})
}
