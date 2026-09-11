import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerAnalyticsRoutes } = await import("./analytics.ts");
const { buildTestApp, request } = await import("../test-support.ts");
const { cleanup, createAccount, createUser, findUser } = await import(
  "../test-db/fixtures.ts"
);

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerAnalyticsRoutes);
const track = () => request(app, "POST", "/api/analytics/registration");

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("POST /api/analytics/registration", () => {
  it("初回だけ計測を許可し、2回目以降は止める", async () => {
    await createAccount(owner.id, "credential");

    const first = await track();
    const second = await track();

    expect(first.json()).toEqual({ shouldTrack: true, method: "email" });
    expect(second.json()).toEqual({ shouldTrack: false });
    expect((await findUser(owner.id))?.analyticsSignUpTrackedAt).toBeInstanceOf(Date);
  });

  it("同時に2回来ても、計測を許可するのは1回だけ", async () => {
    const responses = await Promise.all([track(), track(), track()]);

    expect(responses.filter((res) => res.json().shouldTrack)).toHaveLength(1);
  });

  it("登録方法は最初に作られたアカウントで判定する", async () => {
    await createAccount(owner.id, "github", new Date("2026-01-01T00:00:00.000Z"));
    await createAccount(owner.id, "credential", new Date("2026-02-01T00:00:00.000Z"));

    const res = await track();

    expect(res.json()).toEqual({ shouldTrack: true, method: "github" });
  });

  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await track()).statusCode).toBe(401);
  });
});
