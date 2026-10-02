import { createHash } from "node:crypto";
import { OG_IMAGE, SITE_NAME, type PageMeta } from "@/shared/pageMeta";
import { SITE_URL } from "@/shared/site";

// Next.js では generateMetadata と app/sitemap.ts がこれを担っていた。SPA は誰が来ても
// 同じ index.html を返すため、クローラーと SNS が読む head をサーバー側で作り直す。
// 内容は切り替え前の本番が返していたものに合わせてある。

const DEFAULT_DESCRIPTION =
  "学習の開始から時間記録、予定と実績の確認、科目別の振り返りまでをひとつにつなぐ、大学受験生向け学習管理アプリです。";

// 検索結果に出さないページ。Next.js 版では各ページが NOINDEX を上書きしていた。
// robots.txt では止めない（クロールさせて noindex を読ませる方針。app/robots.ts のコメント参照）。
const NOINDEX_PREFIXES = [
  "/login",
  "/signup",
  "/forgot-password",
  "/reset-password",
  "/verify-email",
  "/dashboard",
  "/schedule",
  "/goals",
  "/profile",
  "/explore",
  "/line",
  "/admin",
];

const STATIC_META: Record<string, { title: string; description: string }> = {
  "/": {
    title: `今日の勉強を、合格までの積み重ねに。｜${SITE_NAME}`,
    description: DEFAULT_DESCRIPTION,
  },
  "/terms": {
    title: `利用規約｜${SITE_NAME}`,
    description: "受験マップをご利用いただく際の条件を定めています。",
  },
  "/privacy": {
    title: `プライバシーポリシー｜${SITE_NAME}`,
    description: "受験マップにおける利用者情報の取り扱いについて説明します。",
  },
};

function escapeAttribute(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

/** 記事以外のページの head の中身。 */
export function defaultMeta(pathname: string): PageMeta {
  const preset = STATIC_META[pathname];
  const title = preset?.title ?? SITE_NAME;

  return {
    title,
    description: preset?.description ?? DEFAULT_DESCRIPTION,
    canonical: preset ? `${SITE_URL}${pathname === "/" ? "" : pathname}` : undefined,
    ogTitle: title,
    ogType: "website",
    ogImage: OG_IMAGE,
    noindex: NOINDEX_PREFIXES.some(
      (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`)
    ),
  };
}

/** 記事のページ（/articles/:id）なら記事の ID を返す。 */
export function articleIdFromPath(pathname: string) {
  return /^\/articles\/([^/]+)$/.exec(pathname)?.[1];
}

// Google Analytics のタグ。Next.js では layout.tsx が GA_MEASUREMENT_ID を読んで
// 差し込んでいた。SPA のバンドルに焼き込むと環境ごとに再ビルドが要るので、
// meta と同じくサーバー側で差し込んで実行時の環境変数のまま扱う。
// 画面遷移の計測は GA4 の拡張計測（履歴の変化を自動で拾う）に任せる。
function analyticsInlineScript(id: string) {
  return `
      window.dataLayer = window.dataLayer || [];
      function gtag(){dataLayer.push(arguments);}
      window.gtag = gtag;
      gtag('js', new Date());
      gtag('config', '${id}');
    `;
}

function analyticsTag() {
  const measurementId = process.env.GA_MEASUREMENT_ID;
  if (!measurementId) return "";

  const id = escapeAttribute(measurementId);
  return `<script async src="https://www.googletagmanager.com/gtag/js?id=${id}"></script>
    <script>${analyticsInlineScript(id)}</script>`;
}

// CSP でインラインスクリプトを許すための sha256。'unsafe-inline' で全部を許す代わりに、
// 上の GA4 の1本だけを中身のハッシュで許す（1文字でも違えば実行されない）。
export function inlineScriptHashes() {
  const measurementId = process.env.GA_MEASUREMENT_ID;
  if (!measurementId) return [];

  const script = analyticsInlineScript(escapeAttribute(measurementId));
  return [`'sha256-${createHash("sha256").update(script).digest("base64")}'`];
}

// 画面のエラーの送り先（Grafana Faro の collector URL）。GA4 と同じ理由で実行時に差し込み、
// 画面側（apps/web/src/lib/faro.ts）がこの meta を読んで送信を始める。無ければ送らない。
function faroTag() {
  const collectorUrl = process.env.FARO_COLLECTOR_URL;
  if (!collectorUrl) return "";
  return `<meta name="faro-collector-url" content="${escapeAttribute(collectorUrl)}"/>`;
}

// index.html の <title> を差し替え、</head> の直前に meta を差し込む。
export function injectMeta(html: string, meta: PageMeta) {
  const tags = [
    `<meta name="description" content="${escapeAttribute(meta.description)}"/>`,
    meta.canonical
      ? `<link rel="canonical" href="${escapeAttribute(meta.canonical)}"/>`
      : "",
    meta.noindex ? `<meta name="robots" content="noindex, nofollow"/>` : "",
    `<meta property="og:site_name" content="${SITE_NAME}"/>`,
    `<meta property="og:locale" content="ja_JP"/>`,
    `<meta property="og:title" content="${escapeAttribute(meta.ogTitle)}"/>`,
    `<meta property="og:description" content="${escapeAttribute(meta.description)}"/>`,
    `<meta property="og:type" content="${meta.ogType}"/>`,
    `<meta property="og:image" content="${escapeAttribute(meta.ogImage)}"/>`,
    meta.canonical
      ? `<meta property="og:url" content="${escapeAttribute(meta.canonical)}"/>`
      : "",
    meta.publishedTime
      ? `<meta property="article:published_time" content="${escapeAttribute(meta.publishedTime)}"/>`
      : "",
    meta.modifiedTime
      ? `<meta property="article:modified_time" content="${escapeAttribute(meta.modifiedTime)}"/>`
      : "",
    `<meta name="twitter:card" content="summary_large_image"/>`,
    `<meta name="twitter:title" content="${escapeAttribute(meta.ogTitle)}"/>`,
    `<meta name="twitter:description" content="${escapeAttribute(meta.description)}"/>`,
    `<meta name="twitter:image" content="${escapeAttribute(meta.ogImage)}"/>`,
    analyticsTag(),
    faroTag(),
  ]
    .filter(Boolean)
    .join("\n    ");

  return html
    .replace(/<title>[^<]*<\/title>/, `<title>${escapeAttribute(meta.title)}</title>`)
    .replace("</head>", `  ${tags}\n  </head>`);
}

/** sitemap に載せる記事。ビルドで SSG した記事の meta（dist/ssg/meta.json）から作る。 */
export type SitemapArticle = { pathname: string; lastModified?: string };

// sitemap.xml。app/sitemap.ts と同じ構成（固定ページ＋記事）。
// 記事は SSG した分だけを載せる（JUK-110）。microCMS には問い合わせないので、表示のたびに待たない。
export function buildSitemap(articles: SitemapArticle[]) {
  const entries: {
    url: string;
    lastModified?: string;
    changeFrequency: string;
    priority: number;
  }[] = [
    { url: SITE_URL, changeFrequency: "weekly", priority: 1 },
    { url: `${SITE_URL}/blog`, changeFrequency: "weekly", priority: 0.6 },
    { url: `${SITE_URL}/terms`, changeFrequency: "yearly", priority: 0.3 },
    { url: `${SITE_URL}/privacy`, changeFrequency: "yearly", priority: 0.3 },
  ];

  for (const article of articles) {
    entries.push({
      url: `${SITE_URL}${article.pathname}`,
      lastModified: article.lastModified && new Date(article.lastModified).toISOString(),
      changeFrequency: "monthly",
      priority: 0.5,
    });
  }

  const body = entries
    .map((entry) => {
      const lastModified = entry.lastModified
        ? `\n    <lastmod>${entry.lastModified}</lastmod>`
        : "";
      return `  <url>
    <loc>${escapeAttribute(entry.url)}</loc>${lastModified}
    <changefreq>${entry.changeFrequency}</changefreq>
    <priority>${entry.priority}</priority>
  </url>`;
    })
    .join("\n");

  return `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${body}
</urlset>`;
}
