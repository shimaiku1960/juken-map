import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import Fastify, { type FastifyInstance } from "fastify";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { registerSpa } from "./spa.ts";

// seo.ts は microCMS のクライアントを読み込み時に作る（CI には接続情報が無い）。
// ここで試すパスは記事ではないので、記事の取得は使わない。
vi.mock("@/api/infra/microcms", () => ({
  getBlog: vi.fn(),
  listBlogs: vi.fn(),
}));

const INDEX_HTML =
  '<html><head><title>x</title></head><body><div id="root"></div></body></html>';
const TERMS_HTML =
  '<html><head><title>x</title></head><body><div id="root"><h1>利用規約</h1></div></body></html>';

describe("registerSpa の SSG ページ", () => {
  let root: string;
  let app: FastifyInstance;

  beforeAll(async () => {
    // apps/web のビルド成果物（dist）と同じ形を一時ディレクトリに作る。
    root = mkdtempSync(path.join(tmpdir(), "spa-test-"));
    writeFileSync(path.join(root, "index.html"), INDEX_HTML);
    mkdirSync(path.join(root, "ssg"));
    writeFileSync(path.join(root, "ssg", "terms.html"), TERMS_HTML);

    app = Fastify();
    registerSpa(app, root);
    await app.ready();
  });

  afterAll(async () => {
    await app.close();
    rmSync(root, { recursive: true, force: true });
  });

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
