// DB のユーザーごとに許す操作。本番の RDS のユーザーも、テスト用 DB のユーザーも、ここから作る。
//
// アプリがマスター（admin）で繋ぐと、SQL インジェクションや資格情報の漏えいが1つ起きただけで、
// テーブルの削除や他の DB の読み取りまでできてしまう。役割ごとにユーザーを分け、
// それぞれに要る操作だけを1つの DB に対して許す（セキュリティ基準 F4）。
//
// - app:      アプリの実行時。services の SQL と Better Auth は DML しか使わない
// - migrate:  デプロイ時のマイグレーション（apps/api-go/migrate.go）だけ。GET_LOCK に権限は要らない
// - readonly: 本番の調査用。書き込めない
//
// 権限を変えるときはここを直し、本番へは `pnpm exec tsx db/print-user-grants.ts` の出力を流す。
// テスト（test-db/global-setup.ts と infra/dbUsers.test.ts）も同じ定義を使うので、
// アプリに要る権限が足りなければテストが落ちる。

const DML = ["SELECT", "INSERT", "UPDATE", "DELETE"] as const;

export const DB_USER_PRIVILEGES = {
  app: [...DML],
  migrate: [...DML, "CREATE", "ALTER", "DROP", "INDEX", "REFERENCES"],
  readonly: ["SELECT"],
} as const;

export type DbUserRole = keyof typeof DB_USER_PRIVILEGES;

// 識別子は ? で渡せないので埋め込む。英数字と _ だけに限って、引用符を壊せないようにする。
function identifier(name: string) {
  if (!/^[A-Za-z0-9_]+$/.test(name)) {
    throw new Error(`DB のユーザー名・DB 名に使えない文字があります: ${name}`);
  }
  return name;
}

/**
 * user に role の権限を database だけに付け直す SQL。ユーザーはすでにある前提。
 *
 * いったん全部取り上げてから付けるので、何度流しても、以前に付けた余分な権限は残らない。
 */
export function grantStatements(role: DbUserRole, user: string, database: string): string[] {
  const account = `'${identifier(user)}'@'%'`;
  return [
    `REVOKE ALL PRIVILEGES, GRANT OPTION FROM ${account}`,
    `GRANT ${DB_USER_PRIVILEGES[role].join(", ")} ON \`${identifier(database)}\`.* TO ${account}`,
  ];
}
