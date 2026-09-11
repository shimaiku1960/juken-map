import { execute, select, transaction, type Db } from "@/api/infra/db";
import type {
  LineConnectionRow,
  LineLinkNonceRow,
  LineOAuthAttemptRow,
} from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";

// LINE 連携まわりのデータ操作。HTTP も LINE の API も知らない。
// 外部 API の呼び出しは infra/line.ts と infra/lineLogin.ts が担当する。

const TEN_MINUTES_MS = 10 * 60 * 1000;

/** この LINE ユーザーが既にどれかのアカウントと連携済みかを見る。 */
export function findConnectionByLineUserId(lineUserId: string) {
  return measured("lineConnection.findByLineUserId", async () => {
    const [row] = await select<Pick<LineConnectionRow, "id">>(
      "SELECT id FROM LineConnection WHERE lineUserId = ? LIMIT 1",
      [lineUserId]
    );
    return row ?? null;
  });
}

/**
 * userId の連携を lineUserId に向ける。無ければ作る。
 *
 * INSERT ... ON DUPLICATE KEY UPDATE は使わない。このテーブルは userId と lineUserId の
 * 2つが UNIQUE で、ON DUPLICATE KEY はどちらの重複でも発動する。lineUserId が
 * 別ユーザーの行とぶつかると、エラーにならず、その他人の行を更新してしまう。
 * userId で UPDATE し、1行も変わらなければ INSERT する（Prisma の upsert も MySQL では
 * 「探す → 更新か作成」の同じ流れだった）。INSERT 側で lineUserId がぶつかれば
 * ER_DUP_ENTRY になり、トランザクションごと取り消される。
 */
async function linkConnection(tx: Db, userId: string, lineUserId: string) {
  const now = new Date();
  const updated = await execute(
    "UPDATE LineConnection SET lineUserId = ?, linkedAt = ?, updatedAt = ? WHERE userId = ?",
    [lineUserId, now, now, userId],
    tx
  );
  if (updated.affectedRows === 0) {
    await execute(
      `INSERT INTO LineConnection (userId, lineUserId, linkedAt, createdAt, updatedAt)
       VALUES (?, ?, ?, ?, ?)`,
      [userId, lineUserId, now, now, now],
      tx
    );
  }
}

/** その LINE が、自分以外のアカウントに連携済みか。 */
async function isLinkedToOtherUser(tx: Db, lineUserId: string, userId: string) {
  const [current] = await select<Pick<LineConnectionRow, "userId">>(
    "SELECT userId FROM LineConnection WHERE lineUserId = ?",
    [lineUserId],
    tx
  );
  return current !== undefined && current.userId !== userId;
}

/**
 * LINE 側から届いた nonce を使って連携を確定する。
 *
 * nonce の消費、「その LINE が既に別アカウントに繋がっていないか」の確認、連携の書き込みを
 * ひとつのトランザクションで行う。分けると、確認と書き込みの間に
 * 別の連携が割り込んで上書きされうる。
 */
export function completeAccountLinkByNonce(nonce: string, lineUserId: string) {
  return measured("lineConnection.completeByNonce", () =>
    transaction(async (tx) => {
      // FOR UPDATE で nonce の行を押さえる。同じ nonce が同時に届いても（LINE の再送など）、
      // 2つ目は1つ目の COMMIT を待ち、そのときには行が消えているので期限切れ扱いになる。
      const [linkNonce] = await select<Pick<LineLinkNonceRow, "userId" | "expiresAt">>(
        "SELECT userId, expiresAt FROM LineLinkNonce WHERE nonce = ? FOR UPDATE",
        [nonce],
        tx
      );
      if (!linkNonce || linkNonce.expiresAt <= new Date()) {
        return { status: "expired" as const };
      }

      // nonce は使い捨て。別アカウントに連携済みで断るときも消す。
      await execute("DELETE FROM LineLinkNonce WHERE nonce = ?", [nonce], tx);
      if (await isLinkedToOtherUser(tx, lineUserId, linkNonce.userId)) {
        return { status: "taken" as const };
      }
      await linkConnection(tx, linkNonce.userId, lineUserId);
      return { status: "linked" as const };
    })
  );
}

/** 連携開始用の nonce を1つだけ持たせる（古いものは捨てる）。 */
export function issueLinkNonce(userId: string, nonce: string) {
  return measured("lineLinkNonce.issue", () =>
    transaction(async (tx) => {
      await execute("DELETE FROM LineLinkNonce WHERE userId = ?", [userId], tx);
      const now = new Date();
      await execute(
        "INSERT INTO LineLinkNonce (nonce, userId, expiresAt, createdAt) VALUES (?, ?, ?, ?)",
        [nonce, userId, new Date(now.getTime() + TEN_MINUTES_MS), now],
        tx
      );
    })
  );
}

/**
 * 連携を解除する。LINE 通知の設定も同時に落とす。
 * 連携が消えたのに通知だけ ON のままだと、送り先が無い通知が残る。
 */
export function disconnectLine(userId: string) {
  return measured("lineConnection.disconnect", () =>
    transaction(async (tx) => {
      await execute(
        `UPDATE NotificationPreference
         SET lineMorningEnabled = FALSE, lineEveningEnabled = FALSE, updatedAt = ?
         WHERE userId = ?`,
        [new Date(), userId],
        tx
      );
      await execute("DELETE FROM LineConnection WHERE userId = ?", [userId], tx);
      await execute("DELETE FROM LineLinkNonce WHERE userId = ?", [userId], tx);
      await execute("DELETE FROM LineOAuthAttempt WHERE userId = ?", [userId], tx);
    })
  );
}

/** LINE Login を始めるとき、進行中の試行を1つだけ持たせる。 */
export function startOAuthAttempt(input: {
  userId: string;
  state: string;
  nonce: string;
  codeVerifier: string;
  redirectUri: string;
}) {
  return measured("lineOAuthAttempt.start", () =>
    transaction(async (tx) => {
      await execute("DELETE FROM LineOAuthAttempt WHERE userId = ?", [input.userId], tx);
      const now = new Date();
      await execute(
        `INSERT INTO LineOAuthAttempt
           (state, userId, nonce, codeVerifier, redirectUri, expiresAt, createdAt)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
        [
          input.state,
          input.userId,
          input.nonce,
          input.codeVerifier,
          input.redirectUri,
          new Date(now.getTime() + TEN_MINUTES_MS),
          now,
        ],
        tx
      );
    })
  );
}

export function findOAuthAttempt(state: string) {
  return measured("lineOAuthAttempt.find", async () => {
    const [row] = await select<LineOAuthAttemptRow>(
      `SELECT state, userId, nonce, codeVerifier, redirectUri, expiresAt, createdAt
       FROM LineOAuthAttempt WHERE state = ?`,
      [state]
    );
    return row ?? null;
  });
}

/** 使い終わった／無効だった試行を捨てる。state は使い捨て。 */
export function discardOAuthAttempt(state: string) {
  return measured("lineOAuthAttempt.discard", async () => {
    // 取得と削除の間に別のリクエストが消していても、0行の削除で終わるだけで落ちない。
    await execute("DELETE FROM LineOAuthAttempt WHERE state = ?", [state]);
  });
}

/**
 * LINE Login で本人確認できたユーザーと連携する。
 * 戻り値 false は「その LINE が別アカウントに連携済み」の意味。
 */
export function linkVerifiedLineUser(userId: string, lineUserId: string) {
  return measured("lineConnection.linkVerified", () =>
    transaction(async (tx) => {
      if (await isLinkedToOtherUser(tx, lineUserId, userId)) return false;
      await linkConnection(tx, userId, lineUserId);
      return true;
    })
  );
}
