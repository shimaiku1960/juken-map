import { createHash } from "node:crypto";
import { execute, select } from "@/api/infra/db";
import { logger } from "@/api/observability/logger";

// メール送信の上限（セキュリティ基準 E1）。宛先ごとと全体の両方で数える。
//
// 確認メールの再送とパスワードの再設定は、ログインしていなくても宛先を指定して送らせられる。
// Better Auth の回数制限は IP 単位なので、IP を変えながら同じ人へ送り続けたり（嫌がらせ）、
// いろいろな宛先へ送って Resend の無料枠（1日100通）を使い切らせたりできる。
// 枠を使い切られると、本物の利用者の確認メールも届かなくなる。
//
// 上限を超えたら送らずにログへ残すだけにする。Better Auth はメール送信を待たずに応答する
// （auth.ts の backgroundTasks）ので、送らなかったことは画面の応答からは分からない。
//
// 毎日の学習通知（Go のバッチ）はここを通らない。あちらは配信記録の UNIQUE 制約で
// 「1人・1日・時間帯・経路ごとに1通」に絞られていて、呼べるのはジョブのトークンを持つ者だけ。

export const EMAIL_KINDS = ["verification", "password-reset", "password-changed", "admin-new-user"] as const;
export type EmailKind = (typeof EMAIL_KINDS)[number];

/** 同じ宛先へ1時間に送る数の上限。再設定を何度か頼み直す本人は困らない数にする。 */
export const EMAIL_PER_RECIPIENT_PER_HOUR = 5;
/**
 * アプリ全体で24時間に送る数の上限。Resend の無料枠は1日100通で、毎日の通知（Go）と分け合う。
 * 通知とずれの分として20通を残す。
 */
export const EMAIL_GLOBAL_PER_DAY = 80;

const HOUR_MS = 60 * 60 * 1000;
const DAY_MS = 24 * HOUR_MS;

/** 運営者への通知は宛先が1つなので、宛先ごとの上限にはかけない（全体の数には入れる）。 */
const PER_RECIPIENT_EXEMPT: ReadonlySet<EmailKind> = new Set(["admin-new-user"]);

function recipientHashOf(to: string) {
  return createHash("sha256").update(to.trim().toLowerCase()).digest("hex");
}

/**
 * 送ってよければ記録して true、上限を超えるなら記録せず false を返す。
 *
 * 先に1行入れてから数え、超えていたら自分の行を消す。数えてから入れると、同時に来た送信が
 * どちらも「まだ上限前」を見て通ってしまう（同時なら両方断ることはあるが、超えることは無い）。
 * 時刻はアプリから渡す（DB の NOW() はセッションのタイムゾーンに左右されるため）。
 */
export async function reserveEmailSend(kind: EmailKind, to: string, now = new Date()): Promise<boolean> {
  const recipientHash = recipientHashOf(to);
  await deleteOldRows(now);

  const { insertId } = await execute(
    "INSERT INTO EmailSend (recipientHash, kind, sentAt) VALUES (?, ?, ?)",
    [recipientHash, kind, now]
  );

  const [{ global }] = await select<{ global: number }>(
    "SELECT COUNT(*) AS global FROM EmailSend WHERE sentAt > ?",
    [new Date(now.getTime() - DAY_MS)]
  );
  let reason: "global" | "recipient" | null = global > EMAIL_GLOBAL_PER_DAY ? "global" : null;

  if (!reason && !PER_RECIPIENT_EXEMPT.has(kind)) {
    const [{ recipient }] = await select<{ recipient: number }>(
      "SELECT COUNT(*) AS recipient FROM EmailSend WHERE recipientHash = ? AND sentAt > ? AND kind <> 'admin-new-user'",
      [recipientHash, new Date(now.getTime() - HOUR_MS)]
    );
    if (recipient > EMAIL_PER_RECIPIENT_PER_HOUR) reason = "recipient";
  }

  if (!reason) return true;

  await execute("DELETE FROM EmailSend WHERE id = ?", [insertId]);
  // 宛先そのものはログに出さない（誰が狙われたかは、ハッシュの先頭で突き合わせられれば足りる）。
  logger.warn(
    { kind, reason, recipient: recipientHash.slice(0, 12) },
    "[email-limits] Email not sent: send limit reached."
  );
  return false;
}

/**
 * 1日より古い行を、送るときに少しずつ消す。日時の範囲で DELETE すると索引の隙間までロックし、
 * 同時の INSERT とぶつかりうるので、ロックを取らない SELECT で主キーを拾ってその行だけを消す。
 */
async function deleteOldRows(now: Date) {
  const cutoff = new Date(now.getTime() - DAY_MS);
  const old = await select<{ id: number }>("SELECT id FROM EmailSend WHERE sentAt <= ? LIMIT 100", [cutoff]);
  if (old.length > 0) {
    await execute("DELETE FROM EmailSend WHERE id IN (?)", [old.map((row) => row.id)]);
  }
}
