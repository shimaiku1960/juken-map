import mysql from "mysql2/promise";
import { afterAll, describe, expect, it } from "vitest";
import { testDatabaseAdminUrl, testDatabaseUrl } from "../test-db/config.ts";
import { ensureTestDbUser } from "../test-db/users.ts";
import { grantStatements } from "./dbUsers.ts";

// 本番の DB ユーザーに付ける権限（dbUsers.ts）で、許すことだけができ、それ以外は拒まれることを
// 本物の MySQL で確かめる。app と migrate のユーザーは globalSetup が同じ定義で作っていて、
// ほかのテストはすべて app の権限で動いている（アプリが DML だけで動くことはそちらで確かめている）。

const readonlyUrl = (() => {
  const url = new URL(testDatabaseUrl);
  url.username = "juken_readonly_test";
  url.password = "juken_readonly_test";
  return url.toString();
})();

const admin = await mysql.createConnection(testDatabaseAdminUrl);
await ensureTestDbUser(admin, "readonly", readonlyUrl);

const app = await mysql.createConnection(testDatabaseUrl);
const readonly = await mysql.createConnection(readonlyUrl);

afterAll(async () => {
  await Promise.all([app.end(), readonly.end(), admin.end()]);
});

// 権限が無くて拒まれたこと（ER_TABLEACCESS_DENIED_ERROR など）。SQL の誤りで落ちたのと区別する。
const denied = { code: expect.stringMatching(/ACCESS_DENIED/) };

describe("app（アプリの実行時）", () => {
  it("読み書きと削除はできる", async () => {
    await app.beginTransaction();
    try {
      const [inserted] = await app.query<mysql.ResultSetHeader>(
        "INSERT INTO Tag (name) VALUES (?)",
        ["dbUsers.test"]
      );
      await app.query("UPDATE Tag SET name = ? WHERE id = ?", ["dbUsers.test2", inserted.insertId]);
      const [rows] = await app.query<mysql.RowDataPacket[]>("SELECT name FROM Tag WHERE id = ?", [
        inserted.insertId,
      ]);
      expect(rows[0].name).toBe("dbUsers.test2");
      await app.query("DELETE FROM Tag WHERE id = ?", [inserted.insertId]);
    } finally {
      await app.rollback();
    }
  });

  it.each([
    ["テーブルの作成", "CREATE TABLE dbusers_test (id int PRIMARY KEY)"],
    ["テーブルの変更", "ALTER TABLE Tag ADD COLUMN dbusers_test int"],
    ["テーブルの削除", "DROP TABLE Tag"],
    ["全行の削除（TRUNCATE）", "TRUNCATE TABLE Tag"],
    ["索引の作成", "CREATE INDEX dbusers_test ON Tag (createdAt)"],
    ["DB の作成", "CREATE DATABASE dbusers_test"],
    ["ほかの DB の読み取り", "SELECT user FROM mysql.user"],
  ])("%sは拒まれる", async (_label, sql) => {
    await expect(app.query(sql)).rejects.toMatchObject(denied);
  });
});

describe("readonly（本番の調査用）", () => {
  it("読める", async () => {
    const [rows] = await readonly.query<mysql.RowDataPacket[]>("SELECT COUNT(*) AS n FROM Tag");
    expect(rows[0].n).toBeGreaterThanOrEqual(0);
  });

  it.each([
    ["追加", "INSERT INTO Tag (name) VALUES ('dbUsers.readonly')"],
    ["更新", "UPDATE Tag SET name = name WHERE id = 0"],
    ["削除", "DELETE FROM Tag WHERE id = 0"],
    ["テーブルの作成", "CREATE TABLE dbusers_test (id int PRIMARY KEY)"],
  ])("%sは拒まれる", async (_label, sql) => {
    await expect(readonly.query(sql)).rejects.toMatchObject(denied);
  });
});

describe("grantStatements", () => {
  it("前の権限を取り上げてから、指定の DB だけに付ける", () => {
    expect(grantStatements("app", "juken_app", "juken_map")).toEqual([
      "REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'juken_app'@'%'",
      "GRANT SELECT, INSERT, UPDATE, DELETE ON `juken_map`.* TO 'juken_app'@'%'",
    ]);
  });

  it("ユーザー名や DB 名に引用符などが混ざっていたら SQL を作らない", () => {
    expect(() => grantStatements("app", "x'@'%", "juken_map")).toThrow();
    expect(() => grantStatements("app", "juken_app", "juken_map`.*")).toThrow();
  });
});
