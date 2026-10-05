// SSG：ビルドの最後に、SSG_PATHS のページと microCMS の全記事を HTML に書き出す。
// 1. vite build が作った dist/index.html（JS・CSS の読み込みタグ入り）をひな形にする
// 2. vite build --ssr が作った dist-server/entry-server.mjs で各ページを描く
// 3. できた HTML を dist/ssg/<パス>.html に置き、記事の meta を dist/ssg/meta.json にまとめる
// 置いたファイルはサーバー（apps/api-go/spa.go）が、そのパスへのリクエストに
// index.html の代わりに返す。meta の差し込みもサーバーが行う。
//
// 記事は microCMS で書き換えるので、記事を更新したらデプロイ（このビルド）をやり直して作り直す（JUK-110）。
// 本番のイメージを作るときは SSG_ARTICLES=required を渡し、記事を取れなければビルドを失敗させる
// （記事の無いイメージを出すと、全記事が 404 になる）。CI と手元は接続情報が無ければ記事を飛ばす。
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const webDir = fileURLToPath(new URL("..", import.meta.url));
const distDir = path.join(webDir, "dist");
const ssgDir = path.join(distDir, "ssg");

const { renderPage, renderArticlePage, articleMeta, SSG_PATHS } = await import(
  path.join(webDir, "dist-server", "entry-server.mjs")
);

const template = await readFile(path.join(distDir, "index.html"), "utf8");

await rm(ssgDir, { recursive: true, force: true });

async function writePage(pathname, html) {
  const file = path.join(ssgDir, `${pathname.slice(1)}.html`);
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(file, html);
  console.log(`SSG: ${pathname} → ${path.relative(webDir, file)}（${html.length} 文字）`);
}

for (const pathname of SSG_PATHS) {
  await writePage(pathname, await renderPage(template, pathname));
}

// パス → meta。記事の分だけを持つ（ほかのページの meta はサーバーが決める）。
const meta = {};
for (const article of await fetchArticles()) {
  // ID はファイル名と URL になるので、microCMS の ID の形（英数字・-・_）以外は書き出さない。
  if (!/^[A-Za-z0-9_-]+$/.test(article.id)) {
    console.warn(`SSG: 記事の ID の形が想定外なので飛ばす（${JSON.stringify(article.id)}）`);
    continue;
  }
  const pathname = `/articles/${article.id}`;
  await writePage(pathname, await renderArticlePage(template, article));
  meta[pathname] = articleMeta(article, pathname);
}
await mkdir(ssgDir, { recursive: true });
await writeFile(path.join(ssgDir, "meta.json"), JSON.stringify(meta));

/** microCMS の公開中の記事を全部取る。取れないときは、本番のビルドなら失敗させ、それ以外は空にする。 */
async function fetchArticles() {
  const required = process.env.SSG_ARTICLES === "required";
  const domain = process.env.MICROCMS_SERVICE_DOMAIN;
  const apiKey = process.env.MICROCMS_API_KEY;

  try {
    if (!domain || !apiKey) throw new Error("MICROCMS_SERVICE_DOMAIN と MICROCMS_API_KEY がありません");
    // サブドメインになるので、形の違う値で別のホストへ送らない。
    if (!/^[a-z0-9-]+$/.test(domain)) throw new Error("MICROCMS_SERVICE_DOMAIN の形が想定外です");

    const articles = [];
    const limit = 100; // microCMS の1回の上限
    for (let offset = 0; ; offset += limit) {
      const url = `https://${domain}.microcms.io/api/v1/blogs?limit=${limit}&offset=${offset}`;
      const res = await fetch(url, {
        headers: { "X-MICROCMS-API-KEY": apiKey },
        signal: AbortSignal.timeout(10_000),
      });
      if (!res.ok) throw new Error(`microCMS が ${res.status} を返しました`);
      const { contents, totalCount } = await res.json();
      articles.push(...contents);
      if (contents.length === 0 || articles.length >= totalCount) break;
    }
    console.log(`SSG: microCMS から記事を ${articles.length} 件取った`);
    return articles;
  } catch (error) {
    if (required) throw new Error(`SSG: 記事を取れないのでビルドを止める（${error.message}）`);
    console.warn(`SSG: 記事を取れないので、記事は書き出さない（${error.message}）`);
    return [];
  }
}
