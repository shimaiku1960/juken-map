// 負荷試験用に、ログイン済みのセッションを先に発行する。
//
// HTTPでログインさせると2つの理由で測りたいものが測れない。
//   - ログイン（Go）は同一IPからのサインインを制限する。k6 は1つのIPから来るので
//     本番では起きない 429 が出る。制限を外すのは本番の守りを弱めるので選ばない。
//   - 実際の利用者は毎回ログインし直さない。セッションを持って来訪する。
//     毎回ログインさせると、測っているのがパスワードのハッシュ（Argon2id）の重さになる。
//
// セッションの Cookie は 256 ビットの乱数のトークンで、DB にはその SHA-256 を置く
// （apps/api-go の auth_session.go・auth_token.go と同じ作り方）。
import { createHash, randomBytes } from "node:crypto";
import { SEED_EMAIL_DOMAIN } from "../src/shared/synthetic";
import { execute, runSeed, select } from "./seed-helpers";

const COUNT = Number(process.env.COUNT ?? 500);
const ACTIVE_DAYS = Number(process.env.ACTIVE_DAYS ?? 7);
// Go の sessionCookieName と同じ。手元の http でも同じ名前（ブラウザは localhost を安全な場所として扱う）。
const COOKIE_NAME = "__Host-jm_session";

runSeed(async () => {
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
    "DELETE FROM AuthSession WHERE userAgent = 'juken-map-loadtest' AND userId IN (SELECT id FROM `user` WHERE email LIKE ?)",
    [`%${SEED_EMAIL_DOMAIN}`]
  );

  const now = new Date();
  const expiresAt = new Date(now.getTime() + 7 * 86400000);
  const cookies: string[] = [];
  const values: unknown[][] = [];
  for (const row of rows) {
    const token = randomBytes(32).toString("base64url");
    cookies.push(`${COOKIE_NAME}=${token}`);
    const tokenHash = createHash("sha256").update(token).digest();
    values.push([randomBytes(16).toString("hex"), tokenHash, row.id, now, expiresAt, now, "127.0.0.1", "juken-map-loadtest"]);
  }

  for (let i = 0; i < values.length; i += 200) {
    const chunk = values.slice(i, i + 200);
    await execute(
      // eslint-disable-next-line no-restricted-syntax -- 埋め込むのは件数ぶん並べた ? だけ。値は chunk.flat() で渡す
      `INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt, ipAddress, userAgent)
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
