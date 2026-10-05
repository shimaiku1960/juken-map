import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { beforeAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import type { RouteOptions } from "fastify";

// 全ルートを登録して onRoute で集め、入口の種類（config.access）ごとに、断るべき相手が
// 本当に断られるかを1本ずつ叩いて確かめる（セキュリティ基準 06 の A1・A2・A5・A6）。
// ルートを足せば自動でここの対象に入るので、人が数えて漏らすことがない。
//
// 利用者と管理者の API はすべて Go へ移し、Node から消した（JUK-84）。Node に残るのは公開のルートだけだが、
// 入口の判定（access-control.ts）は残っているので、利用者・管理者の種類はテスト用のルートを足して確かめる。
// Go のルートの同じ確かめは router_test.go（TestRouterAccess）が行う。

vi.mock("./session-store.ts", () => ({ findSession: vi.fn() }));

// microCMS は読み込むだけで API キーを要求し、呼べば外へ出ていくので差し替える。
vi.mock("@/api/infra/microcms", () => ({
  getBlog: vi.fn(),
  listBlogs: vi.fn(),
  isBlogNotFound: vi.fn(() => false),
}));

const { findSession } = await import("./session-store.ts");
const { ACCESS_ENTRY } = await import("./access-control.ts");
const { TWO_FACTOR_REQUIRED } = await import("@/shared/admin");
const { registerRoutes } = await import("./routes/index.ts");
const { registerSpa } = await import("./spa.ts");
const { buildTestApp, request, loggedInSession, demoSession, adminSessionOf } = await import("./test-support.ts");

const getSession = findSession as unknown as Mock;

type Method = Parameters<typeof request>[1];
type Route = { method: Method; url: string; access: string | undefined };

const routes: Route[] = [];
const app = buildTestApp((app) => {
  app.addHook("onRoute", (route: RouteOptions) => {
    for (const method of [route.method].flat()) {
      if (method === "HEAD") continue;
      routes.push({ method: method as Method, url: route.url, access: route.config?.access });
    }
  });
  registerRoutes(app);
  // 利用者・管理者の種類ごとに、読み取りと書き込みを1本ずつ。
  for (const access of ["user", "admin"] as const) {
    app.get(`/test/${access}`, { config: { access } }, async () => ({ ok: true }));
    app.post(`/test/${access}`, { config: { access } }, async () => ({ ok: true }));
  }
});

const adminSession = adminSessionOf(loggedInSession.user);
const bannedSession = { user: { ...loggedInSession.user, bannedAt: new Date() } };

/** `:id` などの path パラメータを埋める。断る判定はハンドラより前なので、値は何でもよい。 */
const urlOf = (route: Route) => route.url.replace(/:\w+/g, "1");
const label = (route: Route) => `${route.method} ${route.url}`;
const isWrite = (route: Route) => route.method !== "GET";
const byAccess = (...access: string[]) =>
  routes.filter((route) => route.access !== undefined && access.includes(route.access));

/** 各ルートを叩き、期待と違う応答だったものを「メソッド パス → ステータス」で並べる。 */
async function mismatches(targets: Route[], expected: (status: number) => boolean) {
  const failed: string[] = [];
  for (const route of targets) {
    const res = await request(app, route.method, urlOf(route));
    if (!expected(res.statusCode)) failed.push(`${label(route)} → ${res.statusCode}`);
  }
  return failed;
}

beforeAll(() => app.ready());

beforeEach(() => {
  getSession.mockReset();
});

describe("A1 入口の一覧", () => {
  it("登録された全ルートに入口の種類が付いている", () => {
    // 0件のまま通る（何も確かめていない）状態を防ぐ。
    expect(routes.length).toBeGreaterThan(4);
    const known = Object.keys(ACCESS_ENTRY);
    expect(routes.filter((route) => !known.includes(route.access ?? "")).map(label)).toEqual([]);
  });

  it("種類の無いルートを足すと起動（ready）に失敗する", async () => {
    const broken = buildTestApp((app) => {
      app.get("/api/forgotten", async () => ({ ok: true }));
    });
    await expect(broken.ready()).rejects.toThrow("GET /api/forgotten");
  });

  it("静的ファイルのルートには公開の種類が付き、起動できる", async () => {
    const root = mkdtempSync(path.join(tmpdir(), "access-spa-"));
    writeFileSync(path.join(root, "index.html"), "<html><head></head><body></body></html>");
    writeFileSync(path.join(root, "robots.txt"), "User-agent: *");
    try {
      const spa = buildTestApp((app) => registerSpa(app, root));
      await spa.ready();
      expect((await request(spa, "GET", "/robots.txt")).statusCode).toBe(200);
      await spa.close();
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});

describe("A2 既定拒否", () => {
  it("利用者と管理者の API は、未ログインなら全件 401", async () => {
    getSession.mockResolvedValue(null);
    const targets = byAccess("user", "admin");
    expect(targets.length).toBe(4);
    expect(await mismatches(targets, (status) => status === 401)).toEqual([]);
  });

  // OAuth の入口（LINE 連携）とジョブの入口（毎日の通知・シミュレーション）は Go へ移り、Node には無い（JUK-84）。
  // 未ログインをログインへ送ること・デモを断ることは Go の line_test.go（TestLineOAuthStart・TestLineOAuthCallbackState）、
  // トークンが無ければ 401 は notifications_test.go（TestCronHandler・TestJobWithoutSecret）が確かめる。

  it("利用を停止された人は、利用者の API で全件 403", async () => {
    getSession.mockResolvedValue(bannedSession);
    expect(await mismatches(byAccess("user"), (status) => status === 403)).toEqual([]);
  });
});

describe("A5 管理機能", () => {
  it("一般の利用者は管理者の API で全件 403", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const targets = byAccess("admin");
    expect(targets.length).toBe(2);
    expect(await mismatches(targets, (status) => status === 403)).toEqual([]);
  });

  it("デモの利用者は管理者の API で全件 403", async () => {
    getSession.mockResolvedValue(demoSession);
    expect(await mismatches(byAccess("admin"), (status) => status === 403)).toEqual([]);
  });
});

describe("B7 管理者の2段階認証", () => {
  it("2段階認証を通していないセッションの管理者は、管理者の API で全件 403 と印を返す", async () => {
    getSession.mockResolvedValue(adminSessionOf(loggedInSession.user, { twoFactorVerified: false }));
    const failed: string[] = [];
    for (const route of byAccess("admin")) {
      const res = await request(app, route.method, urlOf(route));
      const code = (res.json() as { code?: string }).code;
      if (res.statusCode !== 403 || code !== TWO_FACTOR_REQUIRED) {
        failed.push(`${label(route)} → ${res.statusCode} ${code}`);
      }
    }
    expect(byAccess("admin").length).toBe(2);
    expect(failed).toEqual([]);
  });

  it("2段階認証を通したセッションの管理者は、入口では断られない", async () => {
    getSession.mockResolvedValue(adminSession);
    expect(
      await mismatches(byAccess("admin"), (status) => status !== 401 && status !== 403)
    ).toEqual([]);
  });
});

describe("A6 読み取り専用（デモ）", () => {
  it("デモの利用者は、利用者の API の書き込みで全件 403", async () => {
    getSession.mockResolvedValue(demoSession);
    const targets = byAccess("user").filter(isWrite);
    expect(targets.length).toBe(1);
    expect(await mismatches(targets, (status) => status === 403)).toEqual([]);
  });

  it("デモの利用者でも、利用者の API の読み取りは断らない", async () => {
    getSession.mockResolvedValue(demoSession);
    const targets = byAccess("user").filter((route) => !isWrite(route));
    expect(await mismatches(targets, (status) => status !== 401 && status !== 403)).toEqual([]);
  });
});
