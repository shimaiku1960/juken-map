// 負荷試験用に、ログイン済みのセッションを先に発行する。
//
// HTTPでログインさせると2つの理由で測りたいものが測れない。
//   - Better Auth は同一IPからのサインインを制限する。k6 は1つのIPから来るので
//     本番では起きない 429 が出る。制限を外すのは本番の守りを弱めるので選ばない。
//   - 実際の利用者は毎回ログインし直さない。セッションを持って来訪する。
//     毎回ログインさせると、測っているのが scrypt の重さになる。
//
// Better Auth の session_token クッキーは「token.署名」の形。署名は Better Auth 自身の
// makeSignature（BETTER_AUTH_SECRET の HMAC-SHA256）で作るので、ここでも同じ関数を使う。
import { generateId } from "better-auth";
import { makeSignature } from "better-auth/crypto";
import { SEED_EMAIL_DOMAIN } from "../src/shared/synthetic";
import { execute, runSeed, select } from "./seed-helpers";

const COUNT = Number(process.env.COUNT ?? 500);
const ACTIVE_DAYS = Number(process.env.ACTIVE_DAYS ?? 7);
// HTTPS で動く本番（と本番の AMI から複製した試験環境）では、Better Auth が
// __Secure- を前に付けた名前で読む。手元の http では付かない。
const COOKIE_NAME = `${process.env.SECURE_COOKIE === "on" ? "__Secure-" : ""}better-auth.session_token`;

runSeed(async () => {
  const secret = process.env.BETTER_AUTH_SECRET;
  if (!secret) throw new Error("BETTER_AUTH_SECRET が要ります");

  // 毎日来ている層から選ぶ。直近30日まで広げると、登録したてで数件しか持っていない人が
  // 大半になり、一覧APIが実際より軽く見える。
  const rows = await select<{ id: string; email: string; logs: number }>(
    `SELECT u.id, u.email, COUNT(l.id) AS logs
     FROM \`user\` AS u
     JOIN StudyLog AS l ON l.userId = u.id
     WHERE u.email LIKE ?
     GROUP BY u.id
     HAVING MAX(l.date) >= CURDATE() - INTERVAL ? DAY
     ORDER BY u.id ASC
     LIMIT ?`,
    [`%${SEED_EMAIL_DOMAIN}`, ACTIVE_DAYS, COUNT]
  );
  if (rows.length === 0) {
    throw new Error(
      `直近${ACTIVE_DAYS}日に記録がある合成ユーザーが居ません。先に pnpm db:seed:synthetic を実行してください`
    );
  }

  // 前回の試験で作ったセッションは消す（実利用者のセッションには触れない）。
  await execute(
    "DELETE FROM session WHERE userAgent = 'juken-map-loadtest' AND userId IN (SELECT id FROM `user` WHERE email LIKE ?)",
    [`%${SEED_EMAIL_DOMAIN}`]
  );

  const now = new Date();
  const expiresAt = new Date(now.getTime() + 7 * 86400000);
  const cookies: string[] = [];
  const values: unknown[][] = [];
  for (const row of rows) {
    const token = generateId(32);
    const signature = await makeSignature(token, secret);
    cookies.push(`${COOKIE_NAME}=${token}.${encodeURIComponent(signature)}`);
    values.push([generateId(), row.id, expiresAt, token, now, now, "127.0.0.1", "juken-map-loadtest"]);
  }

  for (let i = 0; i < values.length; i += 200) {
    const chunk = values.slice(i, i + 200);
    await execute(
      `INSERT INTO session (id, userId, expiresAt, token, createdAt, updatedAt, ipAddress, userAgent)
       VALUES ${chunk.map(() => "(?, ?, ?, ?, ?, ?, ?, ?)").join(", ")}`,
      chunk.flat()
    );
  }

  const counts = rows.map((row) => Number(row.logs)).sort((a, b) => a - b);
  console.log(
    JSON.stringify({
      count: rows.length,
      logs_per_user: {
        min: counts[0],
        median: counts[Math.floor(counts.length / 2)],
        max: counts[counts.length - 1],
        avg: Math.round(counts.reduce((sum, n) => sum + n, 0) / counts.length),
      },
      cookies,
    })
  );
});
