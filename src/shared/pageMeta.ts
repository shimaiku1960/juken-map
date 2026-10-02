import { SITE_URL } from "./site";

// ページの head（title・description・OGP）の中身。サーバー（apps/api/src/seo.ts）が index.html に差し込む。
// 記事の meta は、ビルドで記事を SSG するとき（apps/web/scripts/prerender.mjs）に作って
// dist/ssg/meta.json に書き出し、サーバーはそれを読むだけにする（JUK-110）。両方から使うので shared に置く。

export const SITE_NAME = "受験マップ";
export const OG_IMAGE = `${SITE_URL}/opengraph-image.png`;

export type PageMeta = {
  title: string;
  description: string;
  canonical?: string;
  ogTitle: string;
  ogType: "website" | "article";
  ogImage: string;
  noindex: boolean;
  publishedTime?: string;
  modifiedTime?: string;
};

/** meta を作るのに使う記事の項目（microCMS の blogs）。 */
export type ArticleForMeta = {
  title: string;
  description?: string;
  content: string;
  eyecatch?: { url: string };
  createdAt: string;
  updatedAt: string;
};

// 記事本文から説明文を作る。app/articles/[id]/page.tsx の createDescription と同じ処理。
function createDescription(article: ArticleForMeta) {
  if (article.description?.trim()) return article.description.trim();

  return article.content
    .replace(/<[^>]*>/g, " ")
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 120);
}

/** 記事のページ（/articles/:id）の head の中身。 */
export function articleMeta(article: ArticleForMeta, pathname: string): PageMeta {
  return {
    title: `${article.title}｜${SITE_NAME}`,
    description: createDescription(article),
    canonical: `${SITE_URL}${pathname}`,
    ogTitle: article.title,
    ogType: "article",
    ogImage: article.eyecatch?.url ?? OG_IMAGE,
    noindex: false,
    publishedTime: article.createdAt,
    modifiedTime: article.updatedAt,
  };
}
