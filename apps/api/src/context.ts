import type { FastifyReply, FastifyRequest } from "fastify";
import { findSession, type Session } from "./session-store.ts";
import { DEMO_EMAIL } from "@/shared/demo";
import { TWO_FACTOR_REQUIRED } from "@/shared/admin";

// ログインの判定を1か所へまとめる。セッションは Go が発行し、ここでは DB から読むだけ（session-store.ts）。

export type { Session };

export function getSession(request: FastifyRequest) {
  return findSession(request.headers.cookie);
}

/**
 * 未認証なら 401 を送って null を返す。access-control.ts のフックが使う。
 * ハンドラからは呼ばず、フックが通したセッションを currentSession で受け取る。
 */
export async function requireSession(
  request: FastifyRequest,
  reply: FastifyReply
): Promise<Session | null> {
  const session = await getSession(request);
  if (!session) {
    reply.code(401).send({ error: "Unauthorized" });
    return null;
  }
  // 停止された利用者。停止時にその人のセッションは消しているので普通はここに来ないが、
  // 消す直前に始まっていたリクエストが残ることはある。bannedAt はセッションと一緒に引いている。
  if (session.user.bannedAt) {
    reply.code(403).send({ error: "このアカウントは利用を停止されています。" });
    return null;
  }
  return session;
}

/**
 * デモアカウントの編集系リクエストを 403 で止める。止めたら true を返す。
 * Next.js 側の demoReadOnlyGuard と同じ判定・同じ文言。
 * あちらは NextResponse を返す形なので、Fastify 用にここで持つ。
 */
export function denyDemoWrite(
  session: { user: { email: string } },
  reply: FastifyReply
): boolean {
  if (session.user.email === DEMO_EMAIL) {
    reply.code(403).send({ error: "デモアカウントは閲覧専用です" });
    return true;
  }
  return false;
}

/**
 * 未認証なら 401、管理者でないか2段階認証を通していなければ 403 を送って null を返す。access-control.ts のフックが使う。
 * role は user テーブルの列で、セッションと一緒に引いている（session-store.ts）。
 * 画面側でもメニューの出し分けに使うが、守るのはここ。
 */
export async function requireAdmin(
  request: FastifyRequest,
  reply: FastifyReply
): Promise<Session | null> {
  const session = await requireSession(request, reply);
  if (!session) return null;
  if (session.user.role !== "admin") {
    reply.code(403).send({ error: "Forbidden" });
    return null;
  }
  // 管理者は2段階認証を通したセッションでだけ通す（AuthSession.mfaVerifiedAt）。
  // 画面は code を見て、設定の手順かログインし直しの案内を出す。
  if (!session.session.twoFactorVerified) {
    reply.code(403).send({
      error: "管理画面を開くには、2段階認証を通してログインしてください。",
      code: TWO_FACTOR_REQUIRED,
    });
    return null;
  }
  return session;
}
