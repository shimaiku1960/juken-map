import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 認証だけ差し替え、DB は本物のテスト用 MySQL に流す。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerNotificationPreferenceRoutes } = await import(
  "./notification-preferences.ts"
);
const { buildTestApp, request, demoSession } = await import("../test-support.ts");
const {
  cleanup,
  createLineConnection,
  createUser,
  findNotificationPreferences,
} = await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerNotificationPreferenceRoutes);

const get = () => request(app, "GET", "/api/notification-preferences");
const put = (body: unknown) =>
  request(app, "PUT", "/api/notification-preferences", body);

const allOff = {
  emailMorningEnabled: false,
  emailEveningEnabled: false,
  lineMorningEnabled: false,
  lineEveningEnabled: false,
};

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("GET /api/notification-preferences", () => {
  it("未設定なら全部OFFを返す", async () => {
    const res = await get();

    expect(res.json()).toEqual(allOff);
  });

  it("保存した設定を返す", async () => {
    await put({ ...allOff, emailEveningEnabled: true });

    const res = await get();

    expect(res.json()).toEqual({ ...allOff, emailEveningEnabled: true });
  });
});

describe("PUT /api/notification-preferences", () => {
  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await put({})).statusCode).toBe(401);
  });

  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue(demoSession);

    const res = await put({ ...allOff, emailMorningEnabled: true });

    expect(res.statusCode).toBe(403);
  });

  it("初回は作り、2回目は同じ行を書き換える", async () => {
    const first = await put({ ...allOff, emailMorningEnabled: true });
    const [created] = await findNotificationPreferences(owner.id);

    const second = await put({ ...allOff, emailEveningEnabled: true });

    expect(first.statusCode).toBe(200);
    expect(first.json()).toEqual({ ...allOff, emailMorningEnabled: true });
    expect(second.json()).toEqual({ ...allOff, emailEveningEnabled: true });

    const rows = await findNotificationPreferences(owner.id);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({ morningEnabled: false, eveningEnabled: true });
    // 作成日時は据え置き、更新日時だけが進む
    expect(rows[0].createdAt).toEqual(created.createdAt);
    expect(rows[0].updatedAt.getTime()).toBeGreaterThanOrEqual(created.updatedAt.getTime());
  });

  it("初回の保存が同時に2回来ても、行は1つだけできる", async () => {
    const responses = await Promise.all([
      put({ ...allOff, emailMorningEnabled: true }),
      put({ ...allOff, emailMorningEnabled: true }),
    ]);

    expect(responses.map((res) => res.statusCode)).toEqual([200, 200]);
    expect(await findNotificationPreferences(owner.id)).toHaveLength(1);
  });

  it("未連携ではLINE通知を有効化できない", async () => {
    const res = await put({ ...allOff, lineMorningEnabled: true });

    expect(res.statusCode).toBe(400);
    expect(await findNotificationPreferences(owner.id)).toHaveLength(0);
  });

  it("LINE連携済みならLINE通知を有効化できる", async () => {
    await createLineConnection(owner.id);

    const res = await put({ ...allOff, lineMorningEnabled: true });

    expect(res.statusCode).toBe(200);
    expect(res.json()).toEqual({ ...allOff, lineMorningEnabled: true });
  });
});
