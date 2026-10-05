// テスト用 DB の接続先。vitest.config と globalSetup の両方から読む。
//
// 既定値はローカルの docker compose と CI の MySQL サービスに共通の、開発用の認証情報。
// 開発中のデータ（juken_map）とは別の DB を使い、テストが消したり足したりしても困らないようにする。

//
// テストのアプリは、本番と同じ権限（db/db-users.ts の app＝DML だけ）のユーザーで繋ぐ。
// アプリがテーブルの作成・変更のような権限の要る SQL を使い始めたら、テストが落ちて気づける。
// マイグレーションは本番と同じく migrate のユーザーで当てる。どちらも globalSetup が作る。
export const testDatabaseUrl =
  process.env.TEST_DATABASE_URL ??
  "mysql://juken_app_test:juken_app_test@127.0.0.1:3306/juken_map_test";

export const testMigrateDatabaseUrl =
  process.env.TEST_MIGRATE_DATABASE_URL ??
  "mysql://juken_migrate_test:juken_migrate_test@127.0.0.1:3306/juken_map_test";

// マイグレーションのテスト（apps/api/migrate_db_test.go）が毎回作り直す DB に、migrate の権限で繋ぐユーザー。
// juken_map_test とは別の DB なので、ここで作っておく。
export const testMigrationsTestDatabaseUrl = (() => {
  const url = new URL(testDatabaseUrl);
  url.username = "juken_migrations_test";
  url.password = "juken_migrations_test";
  url.pathname = "/juken_map_migrate_test";
  return url.toString();
})();

// テスト用 DB とユーザーを作るための管理者接続。
export const testDatabaseAdminUrl =
  process.env.TEST_DATABASE_ADMIN_URL ?? "mysql://root:rootpassword@127.0.0.1:3306";

export const testDatabaseName = new URL(testDatabaseUrl).pathname.slice(1);

// 取り違えの防止。本番や開発の DB に向いていたら、何かする前に止める。
if (!/_test$/.test(testDatabaseName)) {
  throw new Error(
    `テスト用 DB の名前は _test で終わる必要があります（${testDatabaseName}）`
  );
}
if (new URL(testMigrateDatabaseUrl).pathname.slice(1) !== testDatabaseName) {
  throw new Error("TEST_MIGRATE_DATABASE_URL は TEST_DATABASE_URL と同じ DB を指す必要があります");
}
