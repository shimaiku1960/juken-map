import { createHash } from "node:crypto";
import { execute, select } from "@/api/infra/db";

// メールアドレス（アカウント）単位のサインインの回数制限（セキュリティ基準 B4）。
//
// Better Auth の回数制限は「IP＋パス」をキーにするので、IP を変えながら同じアカウントの
// パスワードを試され続けると止まらない。こちらは IP を見ずにメールアドレスだけで数える。
// 両方そろって B4 の「IP 単位とアカウント単位の両方」になる。
//
// 数えるのは失敗ではなく試行。失敗を応答のあとで数えると、同時に投げられた試行が
// 全部「まだ上限前」を見て通ってしまう。試行を先に1つ足してから判定すれば、同時でも
// 上限を超えた分は止まる。ログインできたら（パスワードが合っていたら）数を消す。
//
// 存在しないメールアドレスも同じように数えて同じように止める。数え方を変えると、
// 止まるかどうかで登録の有無が分かってしまう。
//
// 窓が終わるまでは本人もパスワードでは入れない（他人がわざと失敗させて締め出せる）。
// 窓を短めにしてあり、パスワードの再設定と Google / GitHub のログインはこの制限を通らない。

/** 1つの窓で受け付ける試行の数。 */
export const SIGN_IN_MAX_ATTEMPTS = 10;
/** 窓の長さ。最初の試行から数え、過ぎたら数え直す。 */
export const SIGN_IN_WINDOW_MS = 15 * 60 * 1000;

export type SignInAttemptResult =
  | { allowed: true }
  | { allowed: false; retryAfterSeconds: number };

/** 入力されたメールアドレスをそのまま残さないよう、小文字にしてハッシュにする。 */
function emailHashOf(email: string) {
  return createHash("sha256").update(email.trim().toLowerCase()).digest("hex");
}

/**
 * 試行を1つ数え、上限を超えていないかを返す。
 * 時刻はアプリから渡す（DB の NOW() はセッションのタイムゾーンに左右されるため）。
 */
export async function recordSignInAttempt(email: string, now = new Date()): Promise<SignInAttemptResult> {
  const emailHash = emailHashOf(email);
  const windowStartCutoff = new Date(now.getTime() - SIGN_IN_WINDOW_MS);

  // 窓が終わった行は、ここで少しずつ消す（行が溜まり続けないように）。
  // 日時の範囲で DELETE すると索引の隙間までロックし、同時に来たサインインの INSERT と
  // ぶつかりうる。ロックを取らない SELECT で主キーを拾い、その行だけを消す。
  const expired = await select<{ emailHash: string }>(
    "SELECT emailHash FROM SignInAttempt WHERE windowStartedAt <= ? LIMIT 100",
    [windowStartCutoff]
  );
  if (expired.length > 0) {
    await execute("DELETE FROM SignInAttempt WHERE emailHash IN (?) AND windowStartedAt <= ?", [
      expired.map((row) => row.emailHash),
      windowStartCutoff,
    ]);
  }

  // 足すのと窓の切り替えを1文で行う。MySQL は SET を左から順に評価するので、
  // count の IF は更新前の windowStartedAt を見る（順番を入れ替えると壊れる）。
  //
  // 足したあとの数は、別の SELECT ではなく LAST_INSERT_ID(式) でこの文の応答から受け取る。
  // SELECT で読み直すと、同時に走った試行の分まで数えてしまい、上限より多く止める。
  // 新しく行を作ったとき（affectedRows が 1）は UPDATE 側を通らないので、数は 1。
  const result = await execute(
    `INSERT INTO SignInAttempt (emailHash, count, windowStartedAt) VALUES (?, 1, ?)
     ON DUPLICATE KEY UPDATE
       count = LAST_INSERT_ID(IF(windowStartedAt <= ?, 1, count + 1)),
       windowStartedAt = IF(windowStartedAt <= ?, ?, windowStartedAt)`,
    [emailHash, now, windowStartCutoff, windowStartCutoff, now]
  );
  const count = result.affectedRows === 1 ? 1 : result.insertId;
  if (count <= SIGN_IN_MAX_ATTEMPTS) return { allowed: true };

  // 止めるときだけ、いつ窓が終わるかを読む。
  const [row] = await select<{ windowStartedAt: Date }>(
    "SELECT windowStartedAt FROM SignInAttempt WHERE emailHash = ?",
    [emailHash]
  );
  const windowEndsAt = (row?.windowStartedAt.getTime() ?? now.getTime()) + SIGN_IN_WINDOW_MS;
  return { allowed: false, retryAfterSeconds: Math.max(1, Math.ceil((windowEndsAt - now.getTime()) / 1000)) };
}

/** ログインできたら、そのメールアドレスの数を消す。 */
export async function clearSignInAttempts(email: string) {
  await execute("DELETE FROM SignInAttempt WHERE emailHash = ?", [emailHashOf(email)]);
}
