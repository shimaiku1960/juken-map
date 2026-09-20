import type { FastifyReply, FastifyRequest } from "fastify";
import { fromNodeHeaders } from "better-auth/node";
import { auth } from "./auth.ts";
import { DEMO_EMAIL } from "@/shared/demo";

// Next.js の Route Handler が毎回書いていた
//   const session = await auth.api.getSession({ headers: await headers() });
//   if (!session) return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
// を Fastify 向けに1か所へまとめる。
// Next.js との違いは headers の取り方だけ（await headers() → fromNodeHeaders）。

export type Session = NonNullable<Awaited<ReturnType<typeof auth.api.getSession>>>;

export function getSession(request: FastifyRequest) {
  return auth.api.getSession({ headers: fromNodeHeaders(request.headers) });
}

/**
 * 認証必須のハンドラで使う。未認証なら 401 を送って null を返す。
 * 呼び出し側は `if (!session) return;` で抜ける。
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
  // 停止された利用者。停止時にその人の session は消しているので普通はここに来ないが、
  // 消す直前に始まっていたリクエストが残ることはある。bannedAt はセッションに載っている
  // （auth.ts の additionalFields）ので、DB を引き直さずに断れる。
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
 * 管理者専用のハンドラで使う。未認証なら 401、管理者でなければ 403 を送って null を返す。
 * role は user テーブルの列で、Better Auth がセッションに載せてくる（auth.ts の additionalFields）。
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
  return session;
}
