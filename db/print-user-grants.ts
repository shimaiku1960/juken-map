// 本番の DB ユーザーに付ける権限の SQL を出力する（定義は apps/api/src/infra/dbUsers.ts）。
// ユーザーの作成（パスワード）は別に行い、この出力は権限を付け直すときに流す。
//   例) pnpm exec tsx db/print-user-grants.ts > grants.sql
import { grantStatements } from "../apps/api/src/infra/dbUsers";

const DATABASE = "juken_map";
const USERS = [
  ["app", "juken_app"],
  ["migrate", "juken_migrate"],
  ["readonly", "juken_readonly"],
] as const;

for (const [role, user] of USERS) {
  for (const statement of grantStatements(role, user, DATABASE)) {
    console.log(`${statement};`);
  }
}
