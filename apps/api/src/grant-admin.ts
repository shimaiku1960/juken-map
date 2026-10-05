// 管理者ページ（/admin）の権限を付け外しするコマンド。画面からは付けられないので、最初の1人はこれで付ける。
//
//   pnpm admin:grant you@example.com            # 管理者にする（メール確認済みのユーザーだけ）
//   pnpm admin:grant you@example.com --revoke   # 一般ユーザーに戻す
//   pnpm admin:grant --list                     # 管理者の一覧（2段階認証・パスワードの有無つき）
//
// 本番は RDS に外から繋げないので、EC2 で動いている API のコンテナの中で実行する（README 参照）。
// 付け替えたらその人のセッションを消すので、ログインし直したときから新しい権限になる。
import { pool } from "./infra/db.ts";
import { listAdmins, setUserRoleByEmail } from "./services/user-service.ts";

const args = process.argv.slice(2);
const revoke = args.includes("--revoke");
const email = args.find((arg) => !arg.startsWith("--"));

// 管理画面に入るには、ログインして2段階認証を通す必要がある（apps/api-go の router.go の admin）。
if (args.includes("--list")) {
  try {
    const admins = await listAdmins();
    if (admins.length === 0) console.log("管理者はいません");
    for (const admin of admins) {
      const twoFactor = admin.twoFactorEnabled ? "2段階認証: 有効" : "2段階認証: 未設定";
      const passwordState = admin.hasPassword ? "パスワード: あり" : "パスワード: なし";
      console.log(`${admin.email}\t${twoFactor}\t${passwordState}`);
    }
  } finally {
    await pool.end();
  }
  process.exit(0);
}

if (!email) {
  console.error("使い方: pnpm admin:grant <メールアドレス> [--revoke] / pnpm admin:grant --list");
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
    console.log(
      `${email} の role を ${outcome.previous} → ${role} にし、セッションを ${outcome.sessionsRemoved} 件消しました（ログインし直してください）`
    );
  }
} finally {
  await pool.end();
}
