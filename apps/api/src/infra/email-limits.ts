import { createHash } from "node:crypto";
import type { PoolConnection } from "mysql2/promise";
import { execute, pool, select } from "@/api/infra/db";
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

/** 予約を1件ずつ通すための MySQL の名前付きロック。サーバー全体で1つ。 */
export const EMAIL_SEND_LOCK = "juken-map:email-send";
/** ロックを待つ秒数。待ちきれなければ送らない（上限を確かめられないまま送らない）。 */
const LOCK_TIMEOUT_SECONDS = 5;

type BlockReason = "global" | "recipient" | "lock";

/**
 * 送ってよければ記録して true、上限を超えるなら記録せず false を返す。
 *
 * 数えてから入れるまでを、名前付きロック（GET_LOCK）で1件ずつ通す。ロックなしで数えてから入れると、
 * 同時に来た送信がどれも「まだ上限前」を見て上限を超える。以前は先に入れてから数えていたが、
 * それだと同時に来た送信がどれも互いの行を数えて、全部断ることがあった（JUK-107。上限5の宛先へ
 * 10件同時に送ると、0件になることがあった）。メールは1日に数十通なので、1件ずつ通しても詰まらない。
 * ロックは数ミリ秒で解けるので、待ちきれずに断るのは DB がひどく遅いときだけ（reason は lock）。
 * 時刻はアプリから渡す（DB の NOW() はセッションのタイムゾーンに左右されるため）。
 */
export function reserveEmailSend(kind: EmailKind, to: string, now = new Date()): Promise<boolean> {
  return inOrder(() => reserveLocked(kind, recipientHashOf(to), now));
}

let queue: Promise<unknown> = Promise.resolve();

/**
 * このプロセスの中でも1件ずつ並べる。ロック待ちのあいだも接続を1本握るので、並べずに同時に借りると、
 * 送信が一度に来たときにプール（15本）を使い切り、ログインなどほかの処理が止まる。
 * 名前付きロックは、デプロイの切り替えで Node が2つ同時に動くあいだのために残す。
 */
function inOrder<T>(task: () => Promise<T>): Promise<T> {
  const result = queue.then(task, task);
  queue = result.catch(() => undefined);
  return result;
}

async function reserveLocked(kind: EmailKind, recipientHash: string, now: Date): Promise<boolean> {
  // 名前付きロックは接続に付くので、プールから1本借りて、ロックから解放までを同じ接続で流す。
  const connection = await pool.getConnection();
  let locked = false;
  try {
    const [{ acquired }] = await select<{ acquired: unknown }>(
      "SELECT GET_LOCK(?, ?) AS acquired",
      [EMAIL_SEND_LOCK, LOCK_TIMEOUT_SECONDS],
      connection
    );
    locked = Number(acquired) === 1;
    const reason = locked ? await blockReason(kind, recipientHash, now, connection) : "lock";
    if (!reason) {
      await execute(
        "INSERT INTO EmailSend (recipientHash, kind, sentAt) VALUES (?, ?, ?)",
        [recipientHash, kind, now],
        connection
      );
      return true;
    }
    // 宛先そのものはログに出さない（誰が狙われたかは、ハッシュの先頭で突き合わせられれば足りる）。
    logger.warn(
      { kind, reason, recipient: recipientHash.slice(0, 12) },
      "[email-limits] Email not sent: send limit reached."
    );
    return false;
  } finally {
    await releaseConnection(connection, locked);
  }
}

/** 上限に当たっていればその理由、送ってよければ null。ロックを持った接続で呼ぶ。 */
async function blockReason(
  kind: EmailKind,
  recipientHash: string,
  now: Date,
  connection: PoolConnection
): Promise<BlockReason | null> {
  await deleteOldRows(now, connection);

  const [{ global }] = await select<{ global: number }>(
    "SELECT COUNT(*) AS global FROM EmailSend WHERE sentAt > ?",
    [new Date(now.getTime() - DAY_MS)],
    connection
  );
  if (global >= EMAIL_GLOBAL_PER_DAY) return "global";
  if (PER_RECIPIENT_EXEMPT.has(kind)) return null;

  const [{ recipient }] = await select<{ recipient: number }>(
    "SELECT COUNT(*) AS recipient FROM EmailSend WHERE recipientHash = ? AND sentAt > ? AND kind <> 'admin-new-user'",
    [recipientHash, new Date(now.getTime() - HOUR_MS)],
    connection
  );
  return recipient >= EMAIL_PER_RECIPIENT_PER_HOUR ? "recipient" : null;
}

/**
 * ロックを解いて接続をプールへ返す。解けなかった接続はロックを持ったまま残りうるので、
 * プールへ戻さずに捨てる（接続が切れれば MySQL がロックを解く）。
 */
async function releaseConnection(connection: PoolConnection, locked: boolean) {
  if (!locked) {
    connection.release();
    return;
  }
  try {
    await select("SELECT RELEASE_LOCK(?)", [EMAIL_SEND_LOCK], connection);
    connection.release();
  } catch {
    connection.destroy();
  }
}

/**
 * 1日より古い行を、送るときに少しずつ消す。日時の範囲で DELETE すると索引の隙間までロックし、
 * 同時の INSERT とぶつかりうるので、ロックを取らない SELECT で主キーを拾ってその行だけを消す。
 */
async function deleteOldRows(now: Date, connection: PoolConnection) {
  const cutoff = new Date(now.getTime() - DAY_MS);
  const old = await select<{ id: number }>(
    "SELECT id FROM EmailSend WHERE sentAt <= ? LIMIT 100",
    [cutoff],
    connection
  );
  if (old.length > 0) {
    await execute("DELETE FROM EmailSend WHERE id IN (?)", [old.map((row) => row.id)], connection);
  }
}
