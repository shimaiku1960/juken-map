import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";

// 外部境界だけを差し替える（DB・認証）。門番ロジック本体は実物を動かす。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/backend/infra/prisma", () => ({
  prisma: {
    finalGoal: { findMany: vi.fn(), create: vi.fn() },
  },
}));

vi.mock("@/backend/services/goal-service", () => ({
  listGoals: vi.fn(),
}));

const { auth } = await import("../auth.ts");
const { prisma } = await import("@/backend/infra/prisma");
const { listGoals } = await import("@/backend/services/goal-service");
const { Prisma } = await import("@/backend/generated/prisma/client");
const { registerGoalRoutes } = await import("./goals.ts");
const { buildTestApp, request, loggedInSession, demoSession } = await import("../test-support.ts");

const getSession = auth.api.getSession as unknown as Mock;
const create = prisma.finalGoal.create as unknown as Mock;
const list = listGoals as unknown as Mock;

const app = buildTestApp(registerGoalRoutes);

const post = (body: unknown) => request(app, "POST", "/api/goals", body);
const get = () => request(app, "GET", "/api/goals");

beforeEach(() => {
  vi.clearAllMocks();
});

describe("GET /api/goals", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await get();

    expect(res.statusCode).toBe(401);
    expect(list).not.toHaveBeenCalled();
  });

  it("ログイン済みなら自分の志望校を 200 で返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const goals = [{ id: 1, facultyId: 10, userId: "user-1" }];
    list.mockResolvedValue(goals);

    const res = await get();

    expect(res.statusCode).toBe(200);
    expect(res.json()).toEqual(goals);
    // 自分のデータだけを取りに行っている
    expect(list).toHaveBeenCalledWith("user-1");
  });
});

describe("POST /api/goals", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await post({ facultyId: 1 });

    expect(res.statusCode).toBe(401);
    expect(create).not.toHaveBeenCalled();
  });

  it("デモアカウントなら 403 を返す（作成しない）", async () => {
    getSession.mockResolvedValue(demoSession);

    const res = await post({ facultyId: 10 });

    expect(res.statusCode).toBe(403);
    expect(create).not.toHaveBeenCalled();
  });

  it("入力が不正なら 400 を返す（Zod）", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await post({}); // facultyId が無い

    expect(res.statusCode).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("正常なら 201 で作成した志望校を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const created = { id: 5, facultyId: 10, userId: "user-1" };
    create.mockResolvedValue(created);

    const res = await post({ facultyId: 10 });

    expect(res.statusCode).toBe(201);
    expect(res.json()).toEqual(created);
  });

  it("重複登録（P2002）は 409 に変換する", async () => {
    getSession.mockResolvedValue(loggedInSession);
    create.mockRejectedValue(
      new Prisma.PrismaClientKnownRequestError("Unique constraint failed", {
        code: "P2002",
        clientVersion: "7.9.1",
      })
    );

    const res = await post({ facultyId: 10 });

    expect(res.statusCode).toBe(409);
  });

  it("想定外のエラーは握りつぶさず 500 になる", async () => {
    getSession.mockResolvedValue(loggedInSession);
    create.mockRejectedValue(new Error("boom"));

    const res = await post({ facultyId: 10 });

    expect(res.statusCode).toBe(500);
  });
});
