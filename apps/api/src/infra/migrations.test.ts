import { createHash } from "node:crypto";
import { mkdir, mkdtemp, readdir, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import mysql from "mysql2/promise";
import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { testDatabaseAdminUrl, testDatabaseUrl } from "../test-db/config.ts";
import { MIGRATIONS_DIR, applyMigrations } from "./migrations.ts";

// マイグレーションの適用そのものを、本物の MySQL に作る使い捨ての DB で確かめる。
// テスト用 DB（juken_map_test）は他のテストが使っているので、別の DB を毎回作り直す。
const DB_NAME = "juken_map_migrate_test";
const databaseUrl = (() => {
  const url = new URL(testDatabaseUrl);
  url.pathname = `/${DB_NAME}`;
  return url.toString();
})();

const admin = await mysql.createConnection(testDatabaseAdminUrl);
const appUser = decodeURIComponent(new URL(testDatabaseUrl).username);

async function query<T = mysql.RowDataPacket>(sql: string, params: unknown[] = []) {
  const [rows] = await admin.query(sql, params);
  return rows as T[];
}

const ledger = () =>
  query<{ migration_name: string; checksum: string; finished: number; rolled_back: number; logs: string | null }>(
    `SELECT migration_name, checksum, finished_at IS NOT NULL AS finished,
            rolled_back_at IS NOT NULL AS rolled_back, logs
     FROM \`${DB_NAME}\`._prisma_migrations ORDER BY started_at, migration_name`
  );

const tables = async () =>
  (await query<{ name: string }>(
    "SELECT table_name AS name FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name",
    [DB_NAME]
  )).map((row) => row.name);

/** 一時ディレクトリに migrations を作る。{ 名前: SQL } */
async function migrationsDir(files: Record<string, string>) {
  const dir = await mkdtemp(join(tmpdir(), "juken-map-migrations-"));
  for (const [name, sql] of Object.entries(files)) {
    await mkdir(join(dir, name));
    await writeFile(join(dir, name, "migration.sql"), sql);
  }
  return dir;
}

const apply = (migrationsDir?: string, log: (message: string) => void = () => {}) =>
  applyMigrations({ databaseUrl, migrationsDir, log });

beforeEach(async () => {
  await admin.query(`DROP DATABASE IF EXISTS \`${DB_NAME}\``);
  await admin.query(`CREATE DATABASE \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`);
  await admin.query(`GRANT ALL ON \`${DB_NAME}\`.* TO ?@'%'`, [appUser]);
});

afterAll(async () => {
  await admin.query(`DROP DATABASE IF EXISTS \`${DB_NAME}\``);
  await admin.end();
});

describe("applyMigrations", () => {
  it("空の DB に prisma/migrations を名前順にすべて当て、2回目は何も当てない", async () => {
    const names = (await readdir(MIGRATIONS_DIR, { withFileTypes: true }))
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .sort();

    expect(await apply()).toEqual(names);
    expect(await apply()).toEqual([]);

    // 記録は Prisma と同じ形式（checksum は migration.sql の SHA-256）
    const records = await ledger();
    expect(records.map((r) => r.migration_name)).toEqual(names);
    for (const record of records) {
      const sql = await readFile(join(MIGRATIONS_DIR, record.migration_name, "migration.sql"), "utf8");
      expect(record.checksum).toBe(createHash("sha256").update(sql).digest("hex"));
      expect(record.finished).toBe(1);
    }
    expect(await tables()).toEqual(expect.arrayContaining(["user", "session", "StudyPlan", "StudyLog"]));
  });

  it("当て済みの記録があるものは飛ばし、足されたものだけを当てる", async () => {
    const files = {
      "001_a": "CREATE TABLE a (id int PRIMARY KEY);",
      "002_b": "CREATE TABLE b (id int PRIMARY KEY);\nCREATE TABLE b2 (id int PRIMARY KEY);",
    };
    const dir = await migrationsDir(files);
    expect(await apply(dir)).toEqual(["001_a", "002_b"]);

    await mkdir(join(dir, "003_c"));
    await writeFile(join(dir, "003_c", "migration.sql"), "ALTER TABLE a ADD COLUMN note text;");

    expect(await apply(dir)).toEqual(["003_c"]);
    expect(await tables()).toEqual(["_prisma_migrations", "a", "b", "b2"]);
  });

  it("失敗したら記録を残して止まり、直すまで後ろのものも当てない", async () => {
    const dir = await migrationsDir({
      "001_ok": "CREATE TABLE a (id int PRIMARY KEY);",
      "002_broken": "CREATE TABLE b (id int PRIMARY KEY);\nALTER TABLE no_such_table ADD COLUMN x int;",
      "003_later": "CREATE TABLE c (id int PRIMARY KEY);",
    });

    await expect(apply(dir)).rejects.toThrow("002_broken の適用に失敗しました");
    const records = await ledger();
    expect(records.map((r) => [r.migration_name, r.finished])).toEqual([
      ["001_ok", 1],
      ["002_broken", 0],
    ]);
    expect(records[1].logs).toContain("no_such_table");

    // 途中まで当たった（b はできた）状態のまま、次の実行も止まる
    await expect(apply(dir)).rejects.toThrow("途中で失敗したマイグレーションがあります: 002_broken");
    expect(await tables()).not.toContain("c");
  });

  it("取り消した記録（rolled_back_at）のものは、直したあとでもう一度当てられる", async () => {
    const dir = await migrationsDir({
      "001_broken": "ALTER TABLE no_such_table ADD COLUMN x int;",
    });
    await expect(apply(dir)).rejects.toThrow();

    await writeFile(join(dir, "001_broken", "migration.sql"), "CREATE TABLE fixed (id int PRIMARY KEY);");
    await admin.query(
      `UPDATE \`${DB_NAME}\`._prisma_migrations SET rolled_back_at = NOW(3) WHERE migration_name = '001_broken'`
    );

    expect(await apply(dir)).toEqual(["001_broken"]);
    expect((await ledger()).map((r) => [r.finished, r.rolled_back])).toEqual([
      [0, 1],
      [1, 0],
    ]);
  });

  it("同時に2つ走っても、当てるのは1回だけ", async () => {
    const dir = await migrationsDir({
      "001_a": "CREATE TABLE a (id int PRIMARY KEY);",
      "002_b": "CREATE TABLE b (id int PRIMARY KEY);",
    });

    const results = await Promise.all([apply(dir), apply(dir)]);

    expect(results.flat().sort()).toEqual(["001_a", "002_b"]);
    expect(await ledger()).toHaveLength(2);
  });

  it("当てたあとで migration.sql を書き換えたら警告する（流し直しはしない）", async () => {
    const dir = await migrationsDir({ "001_a": "CREATE TABLE a (id int PRIMARY KEY);" });
    await apply(dir);
    await writeFile(join(dir, "001_a", "migration.sql"), "CREATE TABLE a (id bigint PRIMARY KEY);");

    const messages: string[] = [];
    expect(await apply(dir, (message) => messages.push(message))).toEqual([]);
    expect(messages).toEqual([expect.stringContaining("001_a は当てたあとで")]);
  });
});
