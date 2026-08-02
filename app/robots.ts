import type { MetadataRoute } from "next";

import { SITE_URL } from "@/lib/site";

/**
 * robots.txt を生成する。
 *
 * クロールを禁じる（Disallow）のは、ログインしていないと中身が見えないページと
 * API のみ。未ログインで来ると proxy.ts が /login へリダイレクトするため、
 * クローラーがたどっても価値が無く、クロールバジェットの無駄になる。
 *
 * 認証フローのページ（/login, /signup 等）はあえて Disallow しない。
 * robots.txt でクロールを止めるとページ内の noindex が読まれなくなり、
 * 外部リンク経由で URL だけが検索結果に載ることがあるため、
 * これらは「クロールは許可し、noindex を読ませて除外させる」方式にしている。
 */
const robots = (): MetadataRoute.Robots => ({
  rules: {
    userAgent: "*",
    allow: "/",
    disallow: ["/api/", "/dashboard", "/schedule", "/goals", "/profile", "/explore"],
  },
  sitemap: `${SITE_URL}/sitemap.xml`,
  host: SITE_URL,
});

export default robots;
