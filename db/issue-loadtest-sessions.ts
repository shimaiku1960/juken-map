// 負荷試験用に、ログイン済みのセッションを先に発行する。
//
// HTTPでログインさせると2つの理由で測りたいものが測れない。
//   - ログイン（Go）は同一IPからのサインインを制限する。k6 は1つのIPから来るので
//     本番では起きない 429 が出る。制限を外すのは本番の守りを弱めるので選ばない。
//   - 実際の利用者は毎回ログインし直さない。セッションを持って来訪する。
//     毎回ログインさせると、測っているのがパスワードのハッシュ（Argon2id）の重さになる。
//
// セッションはログイン（Go）の本物の関数で作る（scripts/go-devtool.sh sessions、JUK-143）。トークンの形・
// DB に置くハッシュ・期限・Cookie の名前が、ログインしたときと同じになる。ここでは利用者を選ぶだけにする。
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { SEED_EMAIL_DOMAIN } from "../src/shared/synthetic";
import { execute, runSeed, select } from "./seed-helpers";

const COUNT = Number(process.env.COUNT ?? 500);
const ACTIVE_DAYS = Number(process.env.ACTIVE_DAYS ?? 7);
// 試験で作ったセッションの印。次の試験の前に、これの付いたものだけを消す。
const USER_AGENT = "juken-map-loadtest";
const DEVTOOL = fileURLToPath(new URL("../scripts/go-devtool.sh", import.meta.url));

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
    "DELETE FROM AuthSession WHERE userAgent = ? AND userId IN (SELECT id FROM `user` WHERE email LIKE ?)",
    [USER_AGENT, `%${SEED_EMAIL_DOMAIN}`]
  );

  // 利用者 ID を1行に1つ渡すと、Cookie（名前=値）が1行に1つ返る。
  const out = execFileSync("bash", [DEVTOOL, "sessions", USER_AGENT], {
    input: rows.map((row) => row.id).join("\n"),
    encoding: "utf8",
    env: process.env,
    stdio: ["pipe", "pipe", "inherit"],
  });
  const cookies = out.trim().split("\n");
  if (cookies.length !== rows.length) {
    throw new Error(`セッションを ${rows.length} 件頼んで ${cookies.length} 件返ってきました`);
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
