import { createHash, randomUUID } from "node:crypto";
import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import mysql, { type RowDataPacket } from "mysql2/promise";
import { parseDatabaseUrl } from "./db.ts";

// マイグレーション（テーブル定義の変更）を当てる。`prisma migrate deploy` の代わり。
//
// prisma/migrations/<名前>/migration.sql を名前順に見て、まだ当てていないものだけを流す。
// 当てた記録は、Prisma が使っていた表 _prisma_migrations にそのまま書く。
// 本番の DB には Prisma が当てた記録が残っているので、表を引き継げば移し替えは要らない
// （表の名前に prisma が残るのはそのため）。checksum も Prisma と同じ「ファイルの SHA-256」。
//
// 新しいマイグレーションは、prisma/migrations/<日時>_<内容>/migration.sql を手で書いて足す。
// ORM がスキーマの差分から SQL を作ってくれることはもう無い。

export const MIGRATIONS_DIR = fileURLToPath(
  new URL("../../../../prisma/migrations", import.meta.url)
);

// 同時に2つ動かないようにする MySQL のロック名。デプロイが重なっても二重に当てない。
const LOCK_NAME = "juken_map_migrate";
const LOCK_TIMEOUT_SECONDS = 60;

// Prisma が作っていたのと同じ形。新しい DB（テスト・CI）ではここで作る。
const CREATE_LEDGER_SQL = `
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
  ) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
`;

type LedgerRow = RowDataPacket & {
  migration_name: string;
  checksum: string;
  finished_at: Date | null;
  rolled_back_at: Date | null;
};

/**
 * まだ当てていないマイグレーションを名前順に当て、当てたものの名前を返す。
 *
 * MySQL の CREATE TABLE / ALTER TABLE はトランザクションで取り消せない。途中で失敗すると
 * 半分だけ当たった状態が残るので、Prisma と同じく「失敗した」記録を残して止まり、
 * 人が DB を確かめて直すまで次の実行も止める。
 */
export async function applyMigrations(options: {
  databaseUrl: string | undefined;
  migrationsDir?: string;
  log?: (message: string) => void;
}): Promise<string[]> {
  const { databaseUrl, migrationsDir = MIGRATIONS_DIR, log = console.log } = options;
  if (!databaseUrl) throw new Error("DATABASE_URL が設定されていません");

  // migration.sql は複数の文を1ファイルに持つので、この接続だけ複数文の実行を許す。
  // アプリのプールでは許さない（SQL インジェクションの被害を広げないため）。
  const connection = await mysql.createConnection({
    ...parseDatabaseUrl(databaseUrl),
    timezone: "Z",
    multipleStatements: true,
  });

  try {
    const [[lock]] = await connection.query<RowDataPacket[]>(
      "SELECT GET_LOCK(?, ?) AS acquired",
      [LOCK_NAME, LOCK_TIMEOUT_SECONDS]
    );
    if (lock.acquired !== 1) {
      throw new Error(
        `別のマイグレーションが実行中です（${LOCK_TIMEOUT_SECONDS}秒待ってもロックが取れませんでした）`
      );
    }

    try {
      await connection.query(CREATE_LEDGER_SQL);
      const [records] = await connection.query<LedgerRow[]>(
        "SELECT migration_name, checksum, finished_at, rolled_back_at FROM _prisma_migrations"
      );

      const failed = records.filter((r) => r.finished_at === null && r.rolled_back_at === null);
      if (failed.length > 0) {
        throw new Error(
          `途中で失敗したマイグレーションがあります: ${failed.map((r) => r.migration_name).join(", ")}。` +
            "DB の状態を確かめて直し、_prisma_migrations のその行の rolled_back_at（やり直す場合）" +
            "か finished_at（手で当て終えた場合）を埋めてから、もう一度実行してください"
        );
      }

      // 取り消された記録（rolled_back_at あり）は数えない。同じ名前をもう一度当てられる。
      const applied = new Map(
        records
          .filter((r) => r.finished_at !== null && r.rolled_back_at === null)
          .map((r) => [r.migration_name, r.checksum])
      );

      const names = (await readdir(migrationsDir, { withFileTypes: true }))
        .filter((entry) => entry.isDirectory())
        .map((entry) => entry.name)
        .sort();

      const newlyApplied: string[] = [];
      for (const name of names) {
        const sql = await readFile(join(migrationsDir, name, "migration.sql"), "utf8");
        const checksum = createHash("sha256").update(sql).digest("hex");

        if (applied.has(name)) {
          // 当てたあとでファイルを書き換えても DB には反映されない。気づけるよう知らせるだけにする
          // （Prisma の migrate deploy も、当て済みのものは流し直さなかった）。
          if (applied.get(name) !== checksum) {
            log(`警告: ${name} は当てたあとで migration.sql が書き換えられています`);
          }
          continue;
        }

        const id = randomUUID();
        await connection.query(
          `INSERT INTO _prisma_migrations (id, checksum, migration_name, started_at, applied_steps_count)
           VALUES (?, ?, ?, ?, 0)`,
          [id, checksum, name, new Date()]
        );
        try {
          await connection.query(sql);
        } catch (error) {
          await connection.query("UPDATE _prisma_migrations SET logs = ? WHERE id = ?", [
            String(error),
            id,
          ]);
          throw new Error(`${name} の適用に失敗しました`, { cause: error });
        }
        await connection.query(
          "UPDATE _prisma_migrations SET finished_at = ?, applied_steps_count = 1 WHERE id = ?",
          [new Date(), id]
        );
        log(`適用: ${name}`);
        newlyApplied.push(name);
      }
      return newlyApplied;
    } finally {
      await connection.query("SELECT RELEASE_LOCK(?)", [LOCK_NAME]);
    }
  } finally {
    await connection.end();
  }
}
