import type { FastifyInstance, FastifyReply, FastifyRequest } from "fastify";
import { getBlog, isBlogNotFound, listBlogs } from "@/api/infra/microcms";

// ブログは microCMS の API キーをサーバー側で使うため、SPA から直接は叩けない。
// Next.js では Server Component が client を直接呼んでいたので入口が無かった。
// ここで中継し、キーはサーバーに閉じたままにする。
// 認証は不要（公開コンテンツ）。
export function registerBlogRoutes(app: FastifyInstance) {
  app.get("/api/blog", { config: { access: "public" } }, async (request, reply) => {
    try {
      return await listBlogs();
    } catch (error) {
      return upstreamFailed(request, reply, error);
    }
  });

  app.get<{ Params: { id: string } }>("/api/blog/:id", { config: { access: "public" } }, async (request, reply) => {
    try {
      return await getBlog(request.params.id);
    } catch (error) {
      // 404 だけを「記事が無い」として扱う。タイムアウトや microCMS の障害まで
      // 404 にすると、こちらの調査でも利用者の画面でも原因を取り違える。
      if (isBlogNotFound(error)) {
        return reply.code(404).send({ error: "Not found" });
      }
      return upstreamFailed(request, reply, error);
    }
  });
}

/**
 * microCMS 側の失敗は 502 で返す。
 *
 * 何もしないと Fastify が 500 にするが、それだと「こちらのバグ」と区別が付かない。
 * 可観測性で見るエラー率も、自分の不具合と外部の障害が混ざってしまう。
 */
function upstreamFailed(
  request: FastifyRequest,
  reply: FastifyReply,
  error: unknown
) {
  request.log.error({ err: error }, "[blog] microCMS request failed.");
  return reply.code(502).send({ error: "Bad Gateway" });
}
