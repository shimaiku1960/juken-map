package main

// マイグレーション（テーブル定義の変更）を当てる `migrate` コマンド（JUK-125）。
// Node の apps/api/src/infra/migrations.ts から移し、本番で Node を使う場面を無くした。
//
// <MIGRATIONS_DIR>/<名前>/migration.sql を名前順に見て、まだ当てていないものだけを流す。
// 当てた記録は、Prisma が使っていた表 _prisma_migrations にそのまま書く。本番の DB には Prisma と Node が
// 当てた記録が残っているので、表を引き継げば移し替えは要らない（表の名前に prisma が残るのはそのため）。
// checksum も Prisma と同じ「ファイルの SHA-256」。
//
// 本番はデプロイがアプリの起動前に、Go のイメージの1回きりのコンテナで `/api-go migrate` を流す
// （.github/scripts/deploy-ec2.sh）。手元は `pnpm db:migrate`（`pnpm dev` も最初に流す）、CI は E2E の前に流す。
// 新しいマイグレーションは db/migrations/<日時>_<内容>/migration.sql を手で書いて足す。

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// 同時に2つ動かないようにする MySQL のロック名。デプロイが重なっても二重に当てない。
// Node の版と同じ名前なので、切り替えの前後で重なっても取り合いになる。
const (
	migrationLockName           = "juken_map_migrate"
	migrationLockTimeoutSeconds = 60
)

// migrateTimeout は1回の実行にかけてよい時間。ロック待ち（60秒）と、大きな表の ALTER を見込んで長めにする。
const migrateTimeout = 10 * time.Minute

// Prisma が作っていたのと同じ形。新しい DB（テスト・CI）ではここで作る。
const createLedgerSQL = `
  CREATE TABLE IF NOT EXISTS _prisma_migrations (
    id varchar(36) NOT NULL,
    checksum varchar(64) NOT NULL,
    finished_at datetime(3) DEFAULT NULL,
    migration_name varchar(255) NOT NULL,
    logs text,
    rolled_back_at datetime(3) DEFAULT NULL,
    started_at datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    applied_steps_count int unsigned NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
  ) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// runMigrate は `migrate` コマンド。本番のアプリのユーザーはテーブルを作れない（apps/api/src/infra/dbUsers.ts）ので、
// テーブル定義を変えられるユーザーを MIGRATION_DATABASE_URL で渡す。無ければ DATABASE_URL で当てる（手元・CI）。
func runMigrate(stdout, stderr io.Writer) int {
	databaseURL := os.Getenv("MIGRATION_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
	defer cancel()
	applied, err := applyMigrations(ctx, databaseURL, os.Getenv("MIGRATIONS_DIR"), stdout)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(applied) > 0 {
		fmt.Fprintf(stdout, "マイグレーションを%d本当てました\n", len(applied))
	} else {
		fmt.Fprintln(stdout, "当てるマイグレーションはありません（最新です）")
	}
	return 0
}

// applyMigrations は、dir のマイグレーションのうちまだ当てていないものを名前順に当て、当てたものの名前を返す。
//
// MySQL の CREATE TABLE / ALTER TABLE はトランザクションで取り消せない。途中で失敗すると
// 半分だけ当たった状態が残るので、Prisma と同じく「失敗した」記録を残して止まり、
// 人が DB を確かめて直すまで次の実行も止める。
func applyMigrations(ctx context.Context, databaseURL, dir string, log io.Writer) ([]string, error) {
	names, err := migrationNames(dir)
	if err != nil {
		return nil, err
	}

	cfg, err := dbConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	// migration.sql は複数の文を1ファイルに持つので、この接続だけ複数文の実行を許す。
	// アプリのプール（openDB）では許さない（SQL インジェクションの被害を広げないため）。
	cfg.MultiStatements = true
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	// GET_LOCK は接続ごとのロックなので、プールから1本だけ取り出して最後まで同じ接続を使う。
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", migrationLockName, migrationLockTimeoutSeconds).Scan(&acquired); err != nil {
		return nil, err
	}
	if acquired.Int64 != 1 {
		return nil, fmt.Errorf("別のマイグレーションが実行中です（%d秒待ってもロックが取れませんでした）", migrationLockTimeoutSeconds)
	}
	// 接続を閉じればロックも外れるが、ほかの実行を待たせないよう先に返す。ctx が切れていても返せるようにする。
	defer conn.ExecContext(context.WithoutCancel(ctx), "SELECT RELEASE_LOCK(?)", migrationLockName)

	if _, err := conn.ExecContext(ctx, createLedgerSQL); err != nil {
		return nil, err
	}
	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return nil, err
	}

	var newlyApplied []string
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(dir, name, "migration.sql"))
		if err != nil {
			return newlyApplied, err
		}
		sum := sha256.Sum256(content)
		checksum := hex.EncodeToString(sum[:])

		if recorded, ok := applied[name]; ok {
			// 当てたあとでファイルを書き換えても DB には反映されない。気づけるよう知らせるだけにする
			// （Prisma の migrate deploy も Node の版も、当て済みのものは流し直さなかった）。
			if recorded != checksum {
				fmt.Fprintf(log, "警告: %s は当てたあとで migration.sql が書き換えられています\n", name)
			}
			continue
		}

		id := newMigrationID()
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO _prisma_migrations (id, checksum, migration_name, started_at, applied_steps_count)
			 VALUES (?, ?, ?, UTC_TIMESTAMP(3), 0)`,
			id, checksum, name); err != nil {
			return newlyApplied, err
		}
		if _, err := conn.ExecContext(ctx, string(content)); err != nil {
			// 記録に失敗の中身を残す。この UPDATE も失敗したら、元の失敗の方を返す。
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "UPDATE _prisma_migrations SET logs = ? WHERE id = ?", err.Error(), id)
			return newlyApplied, fmt.Errorf("%s の適用に失敗しました: %w", name, err)
		}
		if _, err := conn.ExecContext(ctx,
			"UPDATE _prisma_migrations SET finished_at = UTC_TIMESTAMP(3), applied_steps_count = 1 WHERE id = ?", id); err != nil {
			return newlyApplied, err
		}
		fmt.Fprintf(log, "適用: %s\n", name)
		newlyApplied = append(newlyApplied, name)
	}
	return newlyApplied, nil
}

// migrationNames は dir の下のディレクトリ名を名前順に返す。1つも無ければ誤りにする。
// イメージに db/migrations を入れ忘れたとき、「最新です」と言って古い表のままアプリを起こさないため。
func migrationNames(dir string) ([]string, error) {
	if dir == "" {
		return nil, errors.New("MIGRATIONS_DIR が空です")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s にマイグレーションがありません", dir)
	}
	sort.Strings(names)
	return names, nil
}

// appliedMigrations は当て終えたマイグレーションの名前と checksum を返す。
// 途中で失敗した記録（finished_at も rolled_back_at も空）があれば、人が直すまで止める。
func appliedMigrations(ctx context.Context, conn *sql.Conn) (map[string]string, error) {
	rows, err := conn.QueryContext(ctx,
		"SELECT migration_name, checksum, finished_at IS NOT NULL, rolled_back_at IS NOT NULL FROM _prisma_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]string{}
	var failed []string
	for rows.Next() {
		var name, checksum string
		var finished, rolledBack bool
		if err := rows.Scan(&name, &checksum, &finished, &rolledBack); err != nil {
			return nil, err
		}
		switch {
		case !finished && !rolledBack:
			failed = append(failed, name)
		// 取り消された記録（rolled_back_at あり）は数えない。同じ名前をもう一度当てられる。
		case finished && !rolledBack:
			applied[name] = checksum
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(failed) > 0 {
		return nil, fmt.Errorf("途中で失敗したマイグレーションがあります: %s。"+
			"DB の状態を確かめて直し、_prisma_migrations のその行の rolled_back_at（やり直す場合）"+
			"か finished_at（手で当て終えた場合）を埋めてから、もう一度実行してください", strings.Join(failed, ", "))
	}
	return applied, nil
}

// newMigrationID は記録の id（varchar(36)）。Prisma と Node の版が入れていたのと同じ UUID v4 の形にする。
func newMigrationID() string {
	b := randomBytes(16)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
