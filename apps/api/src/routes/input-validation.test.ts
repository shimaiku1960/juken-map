import { afterAll, beforeAll, describe, expect, it, vi, type Mock } from "vitest";
import type { RouteOptions } from "fastify";

// 全ルートに、形の合わない入力を送っても 500 にならないことを確かめる（入力検証の漏れを見つける）。
// ルートは登録されたものを onRoute で集めるので、ルートを足せば自動でここの対象に入る。
// 1本ずつのテストが「正しい入力で何が起きるか」を見るのに対し、こちらは「検証を書き忘れたルートが無いか」を見る。

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

// microCMS は読み込むだけで API キーを要求し、呼べば外へ出ていくので差し替える。
vi.mock("@/api/infra/microcms", () => ({
  getBlog: vi.fn(),
  listBlogs: vi.fn(),
  isBlogNotFound: vi.fn(() => false),
}));

const { auth } = await import("../auth.ts");
const { registerRoutes } = await import("./index.ts");
const { buildTestApp, request } = await import("../test-support.ts");
const { cleanup, createUser } = await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;

const routes: { method: string; url: string }[] = [];
const app = buildTestApp((app) => {
  app.addHook("onRoute", (route: RouteOptions) => {
    for (const method of [route.method].flat()) {
      if (method !== "HEAD") routes.push({ method, url: route.url });
    }
  });
  registerRoutes(app);
});

type Method = Parameters<typeof request>[1];

/** `:id` などの path パラメータを、同じ値で埋める。 */
function fillParams(url: string, value: string) {
  return url.replace(/:\w+/g, value);
}

beforeAll(async () => {
  await app.ready();
  // 管理者として送る。認証や権限で先に止まると、検証まで届かず何も確かめられない。
  const user = await createUser();
  getSession.mockResolvedValue({ user: { ...user.session.user, role: "admin" } });
});

afterAll(cleanup);

describe("入力検証の漏れ", () => {
  const withParams = () => routes.filter((route) => route.url.includes(":"));
  const withBody = () =>
    routes.filter((route) => ["POST", "PUT", "PATCH"].includes(route.method));

  it("対象のルートが集まっている", () => {
    // 0件のまま全部通る（何も確かめていない）状態を防ぐ。
    expect(withParams().length).toBeGreaterThan(10);
    expect(withBody().length).toBeGreaterThan(10);
  });

  it.each(["abc", "1.5", "0", "-1", "1e3"])(
    "path パラメータが %s でも 500 にならない",
    async (value) => {
      const failed: string[] = [];
      for (const route of withParams()) {
        const res = await request(app, route.method as Method, fillParams(route.url, value));
        if (res.statusCode >= 500) failed.push(`${route.method} ${route.url}`);
      }
      expect(failed).toEqual([]);
    }
  );

  it("形の合わない body でも 500 にならない", async () => {
    // path パラメータは形としては正しい、存在しない ID にして、body の検証まで届かせる。
    const failed: string[] = [];
    for (const route of withBody()) {
      const res = await request(app, route.method as Method, fillParams(route.url, "999999999"), {
        unexpected: [null],
      });
      if (res.statusCode >= 500) failed.push(`${route.method} ${route.url}`);
    }
    // 500 になったルートを全部並べて見せる（1本目で止めない）。
    expect(failed).toEqual([]);
  });
});
