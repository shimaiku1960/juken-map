import type { MetadataRoute } from "next";

import { client, type Blog } from "@/lib/microcms";
import { SITE_URL } from "@/lib/site";

// ブログ記事は microCMS から取るので、ビルド時に固めず毎回取得する。
// （記事一覧 /blog も同じ方針。ビルド時に API キーが無い環境でも壊れない）
export const dynamic = "force-dynamic";

// 検索結果に載せる公開ページ。認証フロー（/login, /signup 等）と
// ログイン後ページは載せない（それぞれ noindex を付けてある）。
const staticEntries: MetadataRoute.Sitemap = [
  { url: SITE_URL, changeFrequency: "weekly", priority: 1 },
  { url: `${SITE_URL}/blog`, changeFrequency: "weekly", priority: 0.6 },
  { url: `${SITE_URL}/terms`, changeFrequency: "yearly", priority: 0.3 },
  { url: `${SITE_URL}/privacy`, changeFrequency: "yearly", priority: 0.3 },
];

const sitemap = async (): Promise<MetadataRoute.Sitemap> => {
  let articles: MetadataRoute.Sitemap = [];

  try {
    const data = await client.getList<Blog>({
      endpoint: "blogs",
      queries: { fields: "id,updatedAt", limit: 100 },
    });
    articles = data.contents.map((blog) => ({
      url: `${SITE_URL}/articles/${blog.id}`,
      lastModified: new Date(blog.updatedAt),
      changeFrequency: "monthly" as const,
      priority: 0.5,
    }));
  } catch {
    // microCMS が落ちていても、固定ページ分の sitemap は返す。
    // ここで throw すると sitemap.xml 全体が 500 になり、クローラーが何も読めなくなる。
  }

  return [...staticEntries, ...articles];
};

export default sitemap;
