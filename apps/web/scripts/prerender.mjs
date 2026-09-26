// SSG：ビルドの最後に、SSG_PATHS のページを HTML に書き出す。
// 1. vite build が作った dist/index.html（JS・CSS の読み込みタグ入り）をひな形にする
// 2. vite build --ssr が作った dist-server/entry-server.js で各ページを描く
// 3. ひな形の空の <div id="root"></div> に描いた HTML を入れ、dist/ssg/<パス>.html に置く
// 置いたファイルは Fastify（apps/api/src/server.ts の registerSpa）が、そのパスへの
// リクエストに index.html の代わりに返す。meta の差し込みは今までどおり Fastify が行う。
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const webDir = fileURLToPath(new URL("..", import.meta.url));
const distDir = path.join(webDir, "dist");
const ssgDir = path.join(distDir, "ssg");
const ROOT_MARKER = '<div id="root"></div>';

const { render, SSG_PATHS } = await import(
  path.join(webDir, "dist-server", "entry-server.js")
);

const template = await readFile(path.join(distDir, "index.html"), "utf8");
if (template.split(ROOT_MARKER).length !== 2) {
  throw new Error(`dist/index.html に ${ROOT_MARKER} がちょうど1つ必要です`);
}

await rm(ssgDir, { recursive: true, force: true });
for (const pathname of SSG_PATHS) {
  const appHtml = await render(pathname);
  const file = path.join(ssgDir, `${pathname.slice(1)}.html`);
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(file, template.replace(ROOT_MARKER, `<div id="root">${appHtml}</div>`));
  console.log(`SSG: ${pathname} → ${path.relative(webDir, file)}（${appHtml.length} 文字）`);
}
