import mysql from "mysql2/promise";
import { applyMigrations } from "../infra/migrations.ts";
import {
  testDatabaseAdminUrl,
  testDatabaseName,
  testDatabaseUrl,
} from "./config.ts";

// テストの前に1回だけ動く（vitest の globalSetup）。
// テスト用 DB が無ければ作り、マイグレーションを最新まで当てる。
export default async function setup() {
  const admin = await mysql.createConnection(testDatabaseAdminUrl).catch((error) => {
    throw new Error(
      "テスト用の MySQL に接続できません。`pnpm dev:infra` で DB コンテナを起動してください。",
      { cause: error }
    );
  });
  try {
    const appUser = decodeURIComponent(new URL(testDatabaseUrl).username);
    // DB 名とユーザー名は ? で渡せない（値ではなく識別子なので）。
    // config.ts で _test で終わることを確かめた固定値だけを埋め込んでいる。
    await admin.query(
      `CREATE DATABASE IF NOT EXISTS \`${testDatabaseName}\`
       CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`
    );
    await admin.query(`GRANT ALL ON \`${testDatabaseName}\`.* TO ?@'%'`, [appUser]);
  } finally {
    await admin.end();
  }

  // 本番と同じマイグレーションを当てるので、テストの DB は本番と同じ形になる。
  await applyMigrations({ databaseUrl: testDatabaseUrl, log: () => {} });
}
