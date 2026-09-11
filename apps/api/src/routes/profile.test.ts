import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerProfileRoutes } = await import("./profile.ts");
const { buildTestApp, request, demoSession } = await import("../test-support.ts");
const { cleanup, createUser, findUser } = await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerProfileRoutes);
const put = (body: unknown) => request(app, "PUT", "/api/profile", body);

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("PUT /api/profile", () => {
  it("ニックネームを保存し、更新後のユーザーを返す", async () => {
    const res = await put({ nickname: "  いくろう  " });

    expect(res.statusCode).toBe(200);
    // 前後の空白は Zod の trim で落ちる
    expect(res.json()).toMatchObject({ id: owner.id, nickname: "いくろう", emailVerified: false });
    expect((await findUser(owner.id))?.nickname).toBe("いくろう");
  });

  it("空のニックネームは400を返し、保存しない", async () => {
    const res = await put({ nickname: "" });

    expect(res.statusCode).toBe(400);
    expect((await findUser(owner.id))?.nickname).toBeNull();
  });

  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue(demoSession);

    expect((await put({ nickname: "demo" })).statusCode).toBe(403);
  });

  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await put({ nickname: "x" })).statusCode).toBe(401);
  });
});
