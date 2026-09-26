import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import type { FastifyInstance } from "fastify";
import fastifyStatic from "@fastify/static";
import { errorBody } from "./error-handling.ts";
import { buildSitemap, injectMeta, metaForPath } from "./seo.ts";
import { isKnownSpaRoute } from "@/shared/routes";

// 本番では SPA のビルド成果物を API と同じプロセスから配る。nginx は :3000 へ丸ごと
// 流すだけなので、本番ホストの設定を触らずに Next.js と入れ替えられる（切り戻しも
// 既存のイメージ単位の自動ロールバックがそのまま効く）。
// 開発では Vite(:5173) が配って /api だけこちらへプロキシするため、ここは通らない。
export function registerSpa(app: FastifyInstance, root: string) {
  app.register(fastifyStatic, {
    root,
    // ワイルドカードを切り、実ファイルが無いものは下の notFound ハンドラへ落とす。
    // 有効なままだと /dashboard のようなクライアント側ルートが 404 になる。
    wildcard: false,
    // "/" に index.html を直接返させない。そのまま返すと meta を差し込めず、
    // 一番 SEO が要るトップページが既定の head のままになる。
    index: false,
    // ssg/ の HTML は下の notFound ハンドラが meta を差し込んで返す。ファイルのまま
    // /ssg/terms.html でも読めると、meta の無い同じページが別 URL にできてしまう。
    allowedPath: (pathName) => !pathName.startsWith("/ssg/"),
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
  const prerendered = readPrerenderedPages(path.join(root, "ssg"));

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
      return reply.code(404).send(errorBody(404, "NOT_FOUND", String(request.id)));
    }

    const pathname = new URL(request.url, "http://localhost").pathname;

    // SPA は何を渡されても index.html を返せてしまうので、App.tsx が持たないパスは
    // 画面（NotFoundPage）と同じ 404 で返す。200 のままだと、ボットのスキャンまで
    // 「正常」に数えられてエラー率が当てにならず、検索エンジンにも soft 404 と見られる。
    // 本文は変えない（SPA が読み込まれて NotFoundPage を描く）。
    if (!isKnownSpaRoute(pathname)) {
      reply.code(404);
    }

    // SPA なのでクローラーと SNS は JS 実行前の HTML しか読まない。
    // Next.js の generateMetadata が担っていた分を、ここで head に差し込む。
    const meta = await metaForPath(pathname);

    reply.type("text/html; charset=utf-8");
    reply.header("Cache-Control", "no-cache");
    // SSG したページは本文入りの HTML を、それ以外は中身が空の index.html を返す。
    return injectMeta(prerendered.get(pathname) ?? indexHtml, meta);
  });
}

// apps/web のビルドが SSG で書き出した HTML（apps/web/scripts/prerender.mjs）を、
// パス → HTML の形で起動時に一度だけ読む。ssg/terms.html は /terms に対応する。
function readPrerenderedPages(dir: string) {
  const pages = new Map<string, string>();
  if (!existsSync(dir)) return pages;
  for (const file of readdirSync(dir, { recursive: true, encoding: "utf-8" })) {
    if (!file.endsWith(".html")) continue;
    const pathname = `/${file.slice(0, -".html".length).split(path.sep).join("/")}`;
    pages.set(pathname, readFileSync(path.join(dir, file), "utf-8"));
  }
  return pages;
}
