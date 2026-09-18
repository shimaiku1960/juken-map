// 管理者ページ（/admin）の権限を付け外しするコマンド。画面からは付けられないので、最初の1人はこれで付ける。
//
//   pnpm admin:grant you@example.com            # 管理者にする（メール確認済みのユーザーだけ）
//   pnpm admin:grant you@example.com --revoke   # 一般ユーザーに戻す
//
// 本番は RDS に外から繋げないので、EC2 で動いている API のコンテナの中で実行する（README 参照）。
// 付け替えは次にセッションを読み直したときに効く（ログインし直すか、セッションの取り直し）。
import { pool } from "./infra/db.ts";
import { setUserRoleByEmail } from "./services/user-service.ts";

const args = process.argv.slice(2);
const revoke = args.includes("--revoke");
const email = args.find((arg) => !arg.startsWith("--"));

if (!email) {
  console.error("使い方: pnpm admin:grant <メールアドレス> [--revoke]");
  process.exit(1);
}

const role = revoke ? "user" : "admin";

try {
  const outcome = await setUserRoleByEmail(email, role);
  if (outcome.result === "not_found") {
    console.error(`${email} のユーザーが見つかりません`);
    process.exitCode = 1;
  } else if (outcome.result === "unverified") {
    console.error(`${email} はメール確認が済んでいないため、管理者にしません`);
    process.exitCode = 1;
  } else {
    console.log(`${email} の role を ${outcome.previous} → ${role} にしました`);
  }
} finally {
  await pool.end();
}
