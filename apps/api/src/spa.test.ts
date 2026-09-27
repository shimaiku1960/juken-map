import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import Fastify, { type FastifyInstance } from "fastify";
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { getBlog } from "@/api/infra/microcms";
import { registerSpa } from "./spa.ts";

// seo.ts は microCMS のクライアントを読み込み時に作る（CI には接続情報が無い）。
// 記事は getBlog の戻り値をテストごとに決める。
vi.mock("@/api/infra/microcms", () => ({
  getBlog: vi.fn(),
  listBlogs: vi.fn(),
  isBlogNotFound: (error: unknown) => (error as { status?: number }).status === 404,
}));

const INDEX_HTML =
  '<html><head><title>x</title></head><body><div id="root"></div></body></html>';
const TERMS_HTML =
  '<html><head><title>x</title></head><body><div id="root"><h1>利用規約</h1></div></body></html>';
// apps/web の entry-server.tsx の代わり。記事のタイトルを見出しにして返すだけ。
// タイトルが「描けない記事」のときは、描画の失敗として例外を投げる。
const FAKE_SSR_ENTRY = `
export async function renderArticlePage(template, blog) {
  if (blog.title === "描けない記事") throw new Error("render failed");
  return template.replace('<div id="root"></div>', '<div id="root"><h1>' + blog.title + '</h1></div>');
}
`;

const blog = {
  id: "abc",
  title: "記事のタイトル",
  content: "<p>本文</p>",
  createdAt: "2026-08-03T00:00:00.000Z",
  updatedAt: "2026-08-03T00:00:00.000Z",
};

let tmp: string;
let app: FastifyInstance;

beforeAll(async () => {
  // apps/web のビルド成果物と同じ形（dist と、その隣の dist-server）を一時ディレクトリに作る。
  tmp = mkdtempSync(path.join(tmpdir(), "spa-test-"));
  const root = path.join(tmp, "dist");
  mkdirSync(path.join(root, "ssg"), { recursive: true });
  writeFileSync(path.join(root, "index.html"), INDEX_HTML);
  writeFileSync(path.join(root, "ssg", "terms.html"), TERMS_HTML);
  mkdirSync(path.join(tmp, "dist-server"));
  writeFileSync(path.join(tmp, "dist-server", "entry-server.mjs"), FAKE_SSR_ENTRY);

  app = Fastify();
  registerSpa(app, root);
  await app.ready();
});

afterAll(async () => {
  await app.close();
  rmSync(tmp, { recursive: true, force: true });
});

beforeEach(() => {
  vi.mocked(getBlog).mockReset();
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

describe("registerSpa の記事の SSR", () => {
  it("記事を1回だけ取り、本文と記事の meta を入れて返す", async () => {
    vi.mocked(getBlog).mockResolvedValue(blog as never);

    const res = await app.inject({ method: "GET", url: "/articles/abc" });

    expect(res.statusCode).toBe(200);
    expect(res.body).toContain("<h1>記事のタイトル</h1>");
    expect(res.body).toContain('<link rel="canonical" href="https://juken-map.com/articles/abc"/>');
    expect(res.headers["server-timing"]).toMatch(/^cms;dur=[\d.]+, render;dur=[\d.]+$/);
    expect(getBlog).toHaveBeenCalledTimes(1);
  });

  it("記事が無いときは 404 を返す（soft 404 にしない）", async () => {
    vi.mocked(getBlog).mockRejectedValue({ status: 404 });

    const res = await app.inject({ method: "GET", url: "/articles/nope" });

    expect(res.statusCode).toBe(404);
    expect(res.body).toContain('<div id="root"></div>');
    expect(getBlog).toHaveBeenCalledTimes(1);
  });

  it("microCMS の障害では、本文無しの HTML を 200 で返し、もう一度は問い合わせない", async () => {
    vi.mocked(getBlog).mockRejectedValue(new Error("timeout"));

    const res = await app.inject({ method: "GET", url: "/articles/abc" });

    expect(res.statusCode).toBe(200);
    expect(res.body).toContain('<div id="root"></div>');
    expect(getBlog).toHaveBeenCalledTimes(1);
  });

  it("描けなかったときは、記事の meta だけ入れた本文無しの HTML を返す", async () => {
    vi.mocked(getBlog).mockResolvedValue({ ...blog, title: "描けない記事" } as never);

    const res = await app.inject({ method: "GET", url: "/articles/abc" });

    expect(res.statusCode).toBe(200);
    expect(res.body).toContain('<div id="root"></div>');
    expect(res.body).toContain('<meta property="og:title" content="描けない記事"/>');
  });
});
