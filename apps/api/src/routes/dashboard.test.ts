import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 認証だけ差し替え、DB は本物のテスト用 MySQL に流す。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerDashboardRoutes } = await import("./dashboard.ts");
const { buildTestApp, request } = await import("../test-support.ts");
const { cleanup, createStudyLog, createStudyPlan, createUser } = await import(
  "../test-db/fixtures.ts"
);

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerDashboardRoutes);
const get = () => request(app, "GET", "/api/dashboard");

const daysFromToday = (days: number) =>
  new Date(Date.now() + days * 24 * 60 * 60 * 1000);

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("GET /api/dashboard", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await get()).statusCode).toBe(401);
  });

  it("実績・予定・日別合計を1回でまとめて返し、期間も一緒に返す", async () => {
    const log = await createStudyLog(owner.id, { date: new Date(), minutes: 45 });
    const plan = await createStudyPlan(owner.id, { date: new Date() });

    const res = await get();

    expect(res.statusCode).toBe(200);
    const body = res.json();
    // 期間はサーバーが決めて返す（画面が組み立てないで済むように）
    expect(body.month).toMatch(/^\d{4}-\d{2}$/);
    expect(body.logRange.from <= body.logRange.to).toBe(true);
    expect(body.planRange.from <= body.planRange.to).toBe(true);
    expect(body.logs.map((l: { id: number }) => l.id)).toContain(log);
    expect(body.plans.map((p: { id: number }) => p.id)).toContain(plan);
    expect(body.dailyMinutes.some((d: { minutes: number }) => d.minutes === 45)).toBe(true);
  });

  it("実績は過去だけ、予定は今週ぶん先まで覆う", async () => {
    // 予定は未来にもあるので、今週ぶんは実績より先まで取る必要がある
    const upcoming = await createStudyPlan(owner.id, { date: daysFromToday(5) });

    const body = (await get()).json();

    expect(body.plans.map((p: { id: number }) => p.id)).toContain(upcoming);
    expect(body.planRange.to >= body.logRange.to).toBe(true);
  });

  it("遠い過去・遠い未来は含まない（全期間は返さない）", async () => {
    const oldLog = await createStudyLog(owner.id, { date: daysFromToday(-200) });
    const farPlan = await createStudyPlan(owner.id, { date: daysFromToday(200) });

    const body = (await get()).json();

    expect(body.logs.map((l: { id: number }) => l.id)).not.toContain(oldLog);
    expect(body.plans.map((p: { id: number }) => p.id)).not.toContain(farPlan);
  });
});
