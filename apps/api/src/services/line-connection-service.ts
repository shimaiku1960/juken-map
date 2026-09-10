import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

// LINE 連携まわりのデータ操作。HTTP も LINE の API も知らない。
// 外部 API の呼び出しは infra/line.ts と infra/lineLogin.ts が担当する。

const TEN_MINUTES_MS = 10 * 60 * 1000;

/** この LINE ユーザーが既にどれかのアカウントと連携済みかを見る。 */
export function findConnectionByLineUserId(lineUserId: string) {
  return measured("lineConnection.findByLineUserId", () =>
    prisma.lineConnection.findUnique({
      where: { lineUserId },
      select: { id: true },
    })
  );
}

/**
 * LINE 側から届いた nonce を使って連携を確定する。
 *
 * 「その LINE が既に別アカウントに繋がっていないか」の確認と upsert を
 * ひとつのトランザクションで行う。分けると、確認と書き込みの間に
 * 別の連携が割り込んで上書きされうる。
 *
 * 戻り値 false は「別のアカウントに連携済みだった」の意味。
 */
export function completeAccountLinkByNonce(nonce: string, lineUserId: string) {
  return measured("lineConnection.completeByNonce", async () => {
    const linkNonce = await prisma.lineLinkNonce.findUnique({ where: { nonce } });
    if (!linkNonce || linkNonce.expiresAt <= new Date()) {
      return { status: "expired" as const };
    }

    const linked = await prisma.$transaction(async (tx) => {
      const current = await tx.lineConnection.findUnique({ where: { lineUserId } });
      if (current && current.userId !== linkNonce.userId) {
        await tx.lineLinkNonce.delete({ where: { nonce } });
        return false;
      }
      await tx.lineConnection.upsert({
        where: { userId: linkNonce.userId },
        create: { userId: linkNonce.userId, lineUserId },
        update: { lineUserId, linkedAt: new Date() },
      });
      await tx.lineLinkNonce.delete({ where: { nonce } });
      return true;
    });

    return { status: linked ? ("linked" as const) : ("taken" as const) };
  });
}

/** 連携開始用の nonce を1つだけ持たせる（古いものは捨てる）。 */
export function issueLinkNonce(userId: string, nonce: string) {
  return measured("lineLinkNonce.issue", () =>
    prisma.$transaction([
      prisma.lineLinkNonce.deleteMany({ where: { userId } }),
      prisma.lineLinkNonce.create({
        data: { nonce, userId, expiresAt: new Date(Date.now() + TEN_MINUTES_MS) },
      }),
    ])
  );
}

/**
 * 連携を解除する。LINE 通知の設定も同時に落とす。
 * 連携が消えたのに通知だけ ON のままだと、送り先が無い通知が残る。
 */
export function disconnectLine(userId: string) {
  return measured("lineConnection.disconnect", () =>
    prisma.$transaction([
      prisma.notificationPreference.updateMany({
        where: { userId },
        data: { lineMorningEnabled: false, lineEveningEnabled: false },
      }),
      prisma.lineConnection.deleteMany({ where: { userId } }),
      prisma.lineLinkNonce.deleteMany({ where: { userId } }),
      prisma.lineOAuthAttempt.deleteMany({ where: { userId } }),
    ])
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
    prisma.$transaction([
      prisma.lineOAuthAttempt.deleteMany({ where: { userId: input.userId } }),
      prisma.lineOAuthAttempt.create({
        data: { ...input, expiresAt: new Date(Date.now() + TEN_MINUTES_MS) },
      }),
    ])
  );
}

export function findOAuthAttempt(state: string) {
  return measured("lineOAuthAttempt.find", () =>
    prisma.lineOAuthAttempt.findUnique({ where: { state } })
  );
}

/** 使い終わった／無効だった試行を捨てる。state は使い捨て。 */
export function discardOAuthAttempt(state: string) {
  return measured("lineOAuthAttempt.discard", () =>
    prisma.lineOAuthAttempt.deleteMany({ where: { state } })
  );
}

/**
 * LINE Login で本人確認できたユーザーと連携する。
 * 戻り値 false は「その LINE が別アカウントに連携済み」の意味。
 */
export function linkVerifiedLineUser(userId: string, lineUserId: string) {
  return measured("lineConnection.linkVerified", () =>
    prisma.$transaction(async (tx) => {
      const current = await tx.lineConnection.findUnique({ where: { lineUserId } });
      if (current && current.userId !== userId) return false;
      await tx.lineConnection.upsert({
        where: { userId },
        create: { userId, lineUserId },
        update: { lineUserId, linkedAt: new Date() },
      });
      return true;
    })
  );
}
