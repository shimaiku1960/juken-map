import Fastify from "fastify";
import { toNodeHandler } from "better-auth/node";
import { auth } from "./auth.ts";
import { registerRoutes } from "./routes/index.ts";

export function buildServer() {
  const app = Fastify({ logger: false });

  // better-auth は Node のリクエストストリームを自分で読む。
  // Fastify は既定で application/json を先に読み切ってしまうため、そのままだと
  // better-auth 側が空のボディを見て 400 になる（Phase 0 で実際に踏んだ）。
  // addContentTypeParser を parseAs: "string" で挟んでも同じくストリームが枯れる。
  //
  // onRequest はボディ解析より前に走るので、ここで hijack すればストリームは
  // 手つかずのまま better-auth へ渡る。
  app.addHook("onRequest", async (request, reply) => {
    if (!request.url.startsWith("/api/auth/")) return;
    reply.hijack();
    await toNodeHandler(auth)(request.raw, reply.raw);
  });

  // Fastify はハンドラに入る前にボディを解析するので、既定のままだと不正な JSON が
  // 認証チェックより先に 400 になる（Next.js は request.json() がハンドラ内なので
  // 401 が先に返る）。認証を先に効かせるため、解析に失敗しても例外にせず
  // body を undefined にして判断をハンドラへ委ねる。各ハンドラは Zod の safeParse で
  // 検証するので、認証を通った正当なリクエストなら undefined は 400 になる。
  app.addContentTypeParser(
    "application/json",
    { parseAs: "string" },
    (_req, body, done) => {
      if (typeof body !== "string" || body.length === 0) return done(null, undefined);
      try {
        done(null, JSON.parse(body));
      } catch {
        // 解析できない場合も 400 にせず undefined を渡す。
        // 認証されていないリクエストが JSON の中身で結果を知ることがないようにする。
        done(null, undefined);
      }
    }
  );

  app.get("/api/health", async () => ({ ok: true }));

  registerRoutes(app);

  return app;
}

const isEntrypoint = process.argv[1]?.endsWith("server.ts");
if (isEntrypoint) {
  const port = Number(process.env.API_PORT ?? 4000);
  const app = buildServer();
  await app.listen({ port, host: "0.0.0.0" });
  console.log(JSON.stringify({ event: "api.listening", port }));
}
