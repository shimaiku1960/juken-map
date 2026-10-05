import type { Connection } from "mysql2/promise";
import { type DbUserRole, grantStatements } from "../db-users";

/**
 * 接続文字列のユーザーを、role の権限だけを持つ状態にする（無ければ作る）。
 * 権限の中身は本番と同じ定義（db/db-users.ts）から作る。
 */
export async function ensureTestDbUser(admin: Connection, role: DbUserRole, databaseUrl: string) {
  const url = new URL(databaseUrl);
  const user = decodeURIComponent(url.username);
  const password = decodeURIComponent(url.password);
  await admin.query("CREATE USER IF NOT EXISTS ?@'%' IDENTIFIED BY ?", [user, password]);
  await admin.query("ALTER USER ?@'%' IDENTIFIED BY ?", [user, password]);
  for (const statement of grantStatements(role, user, url.pathname.slice(1))) {
    await admin.query(statement);
  }
}
