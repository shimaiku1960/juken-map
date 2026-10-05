import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import Fastify, { type FastifyInstance } from "fastify";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { registerSpa } from "./spa.ts";

const INDEX_HTML =
  '<html><head><title>x</title></head><body><div id="root"></div></body></html>';
const TERMS_HTML =
  '<html><head><title>x</title></head><body><div id="root"><h1>利用規約</h1></div></body></html>';
// apps/web/scripts/prerender.mjs が書き出す記事の HTML と meta の代わり。
const ARTICLE_HTML =
  '<html><head><title>x</title></head><body><div id="root"><h1>記事のタイトル</h1></div></body></html>';
const META = {
  "/articles/abc": {
    title: "記事のタイトル｜受験マップ",
    description: "記事の説明",
    canonical: "https://juken-map.com/articles/abc",
    ogTitle: "記事のタイトル",
    ogType: "article",
    ogImage: "https://juken-map.com/opengraph-image.png",
    noindex: false,
    publishedTime: "2026-08-01T00:00:00.000Z",
    modifiedTime: "2026-08-03T00:00:00.000Z",
  },
};

let tmp: string;
let app: FastifyInstance;

beforeAll(async () => {
  // apps/web のビルド成果物（dist）と同じ形を一時ディレクトリに作る。
  tmp = mkdtempSync(path.join(tmpdir(), "spa-test-"));
  const root = path.join(tmp, "dist");
  mkdirSync(path.join(root, "ssg", "articles"), { recursive: true });
  writeFileSync(path.join(root, "index.html"), INDEX_HTML);
  writeFileSync(path.join(root, "ssg", "terms.html"), TERMS_HTML);
  writeFileSync(path.join(root, "ssg", "articles", "abc.html"), ARTICLE_HTML);
  writeFileSync(path.join(root, "ssg", "meta.json"), JSON.stringify(META));

  app = Fastify();
  registerSpa(app, root);
  await app.ready();
});

afterAll(async () => {
  await app.close();
  rmSync(tmp, { recursive: true, force: true });
});

describe("registerSpa の SSG ページ", () => {
  it("SSG したパスには本文入りの HTML を返し、meta も差し込む", async () => {
    const res = await app.inject({ method: "GET", url: "/terms" });

    expect(res.statusCode).toBe(200);
    expect(res.body).toContain("<h1>利用規約</h1>");
    expect(res.body).toContain('<link rel="canonical" href="https://juken-map.com/terms"/>');
  });

  it("SSG していないパスには、今までどおり中身が空の index.html を返す", async () => {
    const res = await app.inject({ method: "GET", url: "/login" });

    expect(res.statusCode).toBe(200);
    expect(res.body).toContain('<div id="root"></div>');
  });

  it("ssg/ のファイルを直接は配らない（meta の無い同じページが別 URL にできるため）", async () => {
    const res = await app.inject({ method: "GET", url: "/ssg/terms.html" });

    expect(res.statusCode).toBe(404);
    expect(res.body).not.toContain("<h1>利用規約</h1>");
  });
});

describe("registerSpa のメールのリンクで開く画面（認証基準 10 の D3）", () => {
  it.each(["/verify-email/confirm?token=abc", "/reset-password?token=abc"])(
    "%s は Referrer-Policy: no-referrer で返す",
    async (url) => {
      const res = await app.inject({ method: "GET", url });

      expect(res.statusCode).toBe(200);
      expect(res.headers["referrer-policy"]).toBe("no-referrer");
    }
  );

  it("ほかの画面には付けない", async () => {
    const res = await app.inject({ method: "GET", url: "/login" });

    expect(res.headers["referrer-policy"]).toBeUndefined();
  });
});

describe("registerSpa の記事（ビルドで SSG、JUK-110）", () => {
  it("作り置いた本文入りの HTML に、ビルドが書き出した記事の meta を入れて返す", async () => {
    const res = await app.inject({ method: "GET", url: "/articles/abc" });

    expect(res.statusCode).toBe(200);
    expect(res.body).toContain("<h1>記事のタイトル</h1>");
    expect(res.body).toContain("<title>記事のタイトル｜受験マップ</title>");
    expect(res.body).toContain('<link rel="canonical" href="https://juken-map.com/articles/abc"/>');
    expect(res.body).toContain('<meta property="og:type" content="article"/>');
  });

  it("SSG に無い記事は 404 を返す（soft 404 にしない）", async () => {
    const res = await app.inject({ method: "GET", url: "/articles/nope" });

    expect(res.statusCode).toBe(404);
    expect(res.body).toContain('<div id="root"></div>');
  });

  it("meta.json は直接は配らない", async () => {
    const res = await app.inject({ method: "GET", url: "/ssg/meta.json" });

    expect(res.statusCode).toBe(404);
  });

  it("sitemap には SSG した記事を更新日時つきで載せる", async () => {
    const res = await app.inject({ method: "GET", url: "/sitemap.xml" });

    expect(res.statusCode).toBe(200);
    expect(res.body).toContain("<loc>https://juken-map.com/articles/abc</loc>");
    expect(res.body).toContain("<lastmod>2026-08-03T00:00:00.000Z</lastmod>");
    expect(res.body).toContain("<loc>https://juken-map.com/terms</loc>");
  });
});
