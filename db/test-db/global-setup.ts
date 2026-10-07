import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import mysql from "mysql2/promise";
import {
  testDatabaseAdminUrl,
  testDatabaseName,
  testDatabaseUrl,
  testMigrateDatabaseUrl,
  testMigrationsTestDatabaseUrl,
} from "./config";
import { ensureTestDbUser } from "./users";

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
    await ensureTestDbUser(admin, "migrate", testMigrationsTestDatabaseUrl);
  } finally {
    await admin.end();
  }

  // 本番と同じマイグレーションを、本番と同じ Go の migrate コマンド（apps/api/internal/migrate、JUK-125）で当てるので、
  // テストの DB は本番と同じ形になる。
  execFileSync("go", ["run", "./cmd/api", "migrate"], {
    cwd: fileURLToPath(new URL("../../apps/api", import.meta.url)),
    env: {
      ...process.env,
      MIGRATION_DATABASE_URL: testMigrateDatabaseUrl,
      MIGRATIONS_DIR: fileURLToPath(new URL("../migrations", import.meta.url)),
    },
    stdio: ["ignore", "ignore", "inherit"],
  });
}
