import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import Fastify, { type FastifyInstance } from "fastify";
import fastifyCompress from "@fastify/compress";
import fastifyStatic from "@fastify/static";
import { toNodeHandler } from "better-auth/node";
import { auth } from "./auth.ts";
import { registerRoutes } from "./routes/index.ts";
import { buildSitemap, injectMeta, metaForPath } from "./seo.ts";

// 本番では SPA のビルド成果物を API と同じプロセスから配る。nginx は :3000 へ丸ごと
// 流すだけなので、本番ホストの設定を触らずに Next.js と入れ替えられる（切り戻しも
// 既存のイメージ単位の自動ロールバックがそのまま効く）。
// 開発では Vite(:5173) が配って /api だけこちらへプロキシするため、ここは通らない。
function registerSpa(app: FastifyInstance, root: string) {
  app.register(fastifyStatic, {
    root,
    // ワイルドカードを切り、実ファイルが無いものは下の notFound ハンドラへ落とす。
    // 有効なままだと /dashboard のようなクライアント側ルートが 404 になる。
    wildcard: false,
    // "/" に index.html を直接返させない。そのまま返すと meta を差し込めず、
    // 一番 SEO が要るトップページが既定の head のままになる。
    index: false,
    setHeaders(res, filePath) {
      // Vite が出す assets/* はファイル名にハッシュが入るので永久キャッシュしてよい。
      // index.html はデプロイのたびに中身が変わるため、必ず再検証させる。
      if (path.basename(filePath) === "index.html") {
        res.header("Cache-Control", "no-cache");
      } else if (filePath.includes(`${path.sep}assets${path.sep}`)) {
        res.header("Cache-Control", "public, max-age=31536000, immutable");
      }
    },
  });

  // index.html は毎リクエスト読まずに一度だけ読む。meta だけ差し替えて返す。
  const indexHtml = readFileSync(path.join(root, "index.html"), "utf-8");

  // sitemap.xml は記事一覧から作るので静的ファイルにできない。
  // robots.txt と OGP 画像は apps/web/public に置いた実ファイルが配られる。
  app.get("/sitemap.xml", async (_request, reply) => {
    reply.type("application/xml; charset=utf-8");
    return buildSitemap();
  });

  app.setNotFoundHandler(async (request, reply) => {
    // API の 404 まで index.html を返すと、JSON を期待しているクライアントが壊れる。
    // 存在しない API は API のまま 404 を返す。
    if (request.url.startsWith("/api/")) {
      return reply.code(404).send({ error: "Not Found" });
    }

    // SPA なのでクローラーと SNS は JS 実行前の HTML しか読まない。
    // Next.js の generateMetadata が担っていた分を、ここで head に差し込む。
    const pathname = new URL(request.url, "http://localhost").pathname;
    const meta = await metaForPath(pathname);

    reply.type("text/html; charset=utf-8");
    reply.header("Cache-Control", "no-cache");
    return injectMeta(indexHtml, meta);
  });
}

export async function buildServer() {
  const app = Fastify({ logger: false });

  // Next.js は応答を既定で圧縮していたが、Fastify は何もしない。SPA のバンドルは
  // 800KB 超あり、無圧縮のまま配ると回線の細い端末で目に見えて遅くなる。
  // nginx 側で gzip を足す手もあるが、本番ホストを触らない方針なのでアプリで持つ。
  //
  // ここは必ず await する。register() は遅延実行なので、待たずに下でルートを
  // 定義すると、プラグインが読み込まれる頃には既にルートが確定していて hook が
  // 掛からない。実際それで静的ファイル以外が無圧縮のままだった。
  await app.register(fastifyCompress, {
    global: true,
    encodings: ["br", "gzip", "deflate"],
  });

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

  const webDist = process.env.WEB_DIST_DIR;
  if (webDist && existsSync(path.join(webDist, "index.html"))) {
    registerSpa(app, webDist);
  }

  return app;
}

const isEntrypoint = process.argv[1]?.endsWith("server.ts");
if (isEntrypoint) {
  const port = Number(process.env.API_PORT ?? 4000);
  const app = await buildServer();
  await app.listen({ port, host: "0.0.0.0" });
  console.log(JSON.stringify({ event: "api.listening", port }));
}
