// テスト用 DB の接続先。vitest.config と globalSetup の両方から読む。
//
// 既定値はローカルの docker compose と CI の MySQL サービスに共通の、開発用の認証情報。
// 開発中のデータ（juken_map）とは別の DB を使い、テストが消したり足したりしても困らないようにする。

export const testDatabaseUrl =
  process.env.TEST_DATABASE_URL ??
  "mysql://juken:jukenpassword@127.0.0.1:3306/juken_map_test";

// テスト用 DB を作るための管理者接続。アプリ用ユーザー（juken）には CREATE DATABASE の権限が無い。
export const testDatabaseAdminUrl =
  process.env.TEST_DATABASE_ADMIN_URL ?? "mysql://root:rootpassword@127.0.0.1:3306";

export const testDatabaseName = new URL(testDatabaseUrl).pathname.slice(1);

// 取り違えの防止。本番や開発の DB に向いていたら、何かする前に止める。
if (!/_test$/.test(testDatabaseName)) {
  throw new Error(
    `テスト用 DB の名前は _test で終わる必要があります（${testDatabaseName}）`
  );
}
