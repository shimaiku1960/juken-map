// 乗っ取りが起きたときの操作をまとめたコマンド。手順は docs/incident-response.md。
//
//   pnpm incident sessions <メールアドレス>    # ログイン中のセッション（いつ・どこから）を見る
//   pnpm incident revoke <メールアドレス>      # セッションをすべて消す（止めはしない）
//   pnpm incident ban <メールアドレス>         # 止めて、セッションをすべて消す（管理者も止められる）
//   pnpm incident unban <メールアドレス>       # 止めたのを戻す
//   pnpm incident revoke-admins               # 管理者全員のセッションを消す
//   pnpm incident revoke-all                  # 全員のセッションを消す（全員がログインし直し）
//   pnpm incident reset-2fa <メールアドレス>   # 2段階認証を設定前に戻し、セッションをすべて消す
//
// 本番は RDS に外から繋げないので、pnpm admin:grant と同じく EC2 の API のコンテナの中で実行する。
import { pool } from "./infra/db.ts";
import {
  banByEmail,
  listSessionsByEmail,
  resetTwoFactorByEmail,
  revokeAdminSessions,
  revokeAllSessions,
  revokeSessionsByEmail,
  unbanByEmail,
} from "./services/incident-service.ts";

const USAGE = `使い方: pnpm incident <操作> [メールアドレス]
  sessions <メール>   ログイン中のセッションを見る
  revoke <メール>     セッションをすべて消す
  ban <メール>        止めて、セッションをすべて消す
  unban <メール>      止めたのを戻す
  revoke-admins       管理者全員のセッションを消す
  revoke-all          全員のセッションを消す（全員がログインし直し）
  reset-2fa <メール>  2段階認証を設定前に戻し、セッションをすべて消す`;

const [command, email] = process.argv.slice(2);

function notFound() {
  console.error(`${email} のユーザーが見つかりません`);
  process.exitCode = 1;
}

async function run() {
  if (command === "revoke-admins") {
    const { admins, sessionsRemoved } = await revokeAdminSessions();
    console.log(`管理者 ${admins.length} 人のセッションを ${sessionsRemoved} 件消しました`);
    for (const admin of admins) console.log(`  ${admin.email ?? admin.id}`);
    return;
  }

  if (command === "revoke-all") {
    const sessionsRemoved = await revokeAllSessions();
    console.log(`全員のセッションを ${sessionsRemoved} 件消しました`);
    return;
  }

  if (!email) {
    console.error(USAGE);
    process.exitCode = 1;
    return;
  }

  switch (command) {
    case "sessions": {
      const found = await listSessionsByEmail(email);
      if (!found) return notFound();
      const banned = found.user.bannedAt ? `（停止中: ${found.user.bannedAt.toISOString()}）` : "";
      console.log(`${email}  role=${found.user.role}${banned}  セッション ${found.sessions.length} 件`);
      for (const s of found.sessions) {
        const twoFactor = s.twoFactorVerified ? "2FA済み" : "2FAなし";
        console.log(
          `  作成 ${s.createdAt.toISOString()}  最終 ${s.lastUsedAt.toISOString()}  期限 ${s.expiresAt.toISOString()}  ${twoFactor}  ${s.ipAddress ?? "-"}  ${s.userAgent ?? "-"}`
        );
      }
      return;
    }
    case "revoke": {
      const done = await revokeSessionsByEmail(email);
      if (!done) return notFound();
      console.log(`${email} のセッションを ${done.sessionsRemoved} 件消しました`);
      return;
    }
    case "ban": {
      const done = await banByEmail(email);
      if (!done) return notFound();
      console.log(`${email} を止め、セッションを ${done.sessionsRemoved} 件消しました`);
      return;
    }
    case "unban": {
      if (!(await unbanByEmail(email))) return notFound();
      console.log(`${email} の停止を戻しました`);
      return;
    }
    case "reset-2fa": {
      const done = await resetTwoFactorByEmail(email);
      if (!done) return notFound();
      console.log(
        `${email} の2段階認証を設定前に戻し、セッションを ${done.sessionsRemoved} 件消しました`
      );
      return;
    }
    default:
      console.error(USAGE);
      process.exitCode = 1;
  }
}

try {
  await run();
} finally {
  await pool.end();
}
