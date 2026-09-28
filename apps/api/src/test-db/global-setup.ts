import mysql from "mysql2/promise";
import { applyMigrations } from "../infra/migrations.ts";
import {
  testDatabaseAdminUrl,
  testDatabaseName,
  testDatabaseUrl,
  testMigrateDatabaseUrl,
} from "./config.ts";
import { ensureTestDbUser } from "./users.ts";

// テストの前に1回だけ動く（vitest の globalSetup）。
// テスト用 DB が無ければ作り、本番と同じ権限のユーザーを用意して、マイグレーションを最新まで当てる。
export default async function setup() {
  const admin = await mysql.createConnection(testDatabaseAdminUrl).catch((error) => {
    throw new Error(
      "テスト用の MySQL に接続できません。`pnpm db:start` で DB コンテナを起動してください。",
      { cause: error }
    );
  });
  try {
    // DB 名は ? で渡せない（値ではなく識別子なので）。
    // config.ts で _test で終わることを確かめた固定値だけを埋め込んでいる。
    await admin.query(
      `CREATE DATABASE IF NOT EXISTS \`${testDatabaseName}\`
       CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`
    );
    await ensureTestDbUser(admin, "app", testDatabaseUrl);
    await ensureTestDbUser(admin, "migrate", testMigrateDatabaseUrl);
  } finally {
    await admin.end();
  }

  // 本番と同じマイグレーションを当てるので、テストの DB は本番と同じ形になる。
  await applyMigrations({ databaseUrl: testMigrateDatabaseUrl, log: () => {} });
}
