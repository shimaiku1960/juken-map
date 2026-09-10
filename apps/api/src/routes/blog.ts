import type { FastifyInstance } from "fastify";
import { client, type Blog } from "@/api/infra/microcms";

// ブログは microCMS の API キーをサーバー側で使うため、SPA から直接は叩けない。
// Next.js では Server Component が client を直接呼んでいたので入口が無かった。
// ここで中継し、キーはサーバーに閉じたままにする。
// 認証は不要（公開コンテンツ）。
export function registerBlogRoutes(app: FastifyInstance) {
  app.get("/api/blog", async () => {
    const data = await client.getList<Blog>({ endpoint: "blogs" });
    return data;
  });

  app.get<{ Params: { id: string } }>("/api/blog/:id", async (request, reply) => {
    try {
      return await client.get<Blog>({
        endpoint: "blogs",
        contentId: request.params.id,
      });
    } catch {
      // microCMS は存在しない contentId で 404 を投げる。
      return reply.code(404).send({ error: "Not found" });
    }
  });
}
