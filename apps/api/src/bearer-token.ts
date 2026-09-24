import { createHash, timingSafeEqual } from "node:crypto";
import type { FastifyRequest } from "fastify";

// GitHub Actions から呼ばれる入口（cron・シミュレーション）の共有トークンを確かめる。
// セッションではなく `Authorization: Bearer <秘密値>` で守る。
//
// 文字列を === で比べると、先頭から何文字一致したかで返るまでの時間が変わり、
// 外から1文字ずつ当てられる余地が残る。両方を SHA-256 にそろえてから
// timingSafeEqual で比べる。ハッシュにするのは、timingSafeEqual が同じ長さしか
// 受け付けず、長さの違いで先に返すと秘密値の長さが漏れるため。

function digest(value: string) {
  return createHash("sha256").update(value).digest();
}

/**
 * `Authorization: Bearer <secret>` が一致すれば true。
 * secret が未設定（undefined・空文字）なら、何が送られても false にする。
 */
export function hasBearerToken(request: FastifyRequest, secret: string | undefined) {
  if (!secret) return false;
  const header = request.headers.authorization;
  if (typeof header !== "string") return false;
  return timingSafeEqual(digest(header), digest(`Bearer ${secret}`));
}
