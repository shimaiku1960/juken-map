import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";

// 差し替えるのは外部境界（DB・認証）だけ。routes と services は実物を動かし、
// 「どんな SQL を投げたか」で振る舞いを確かめる。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/api/infra/prisma", () => {
  // routes は名前付き、services は default で import している。
  // 同じ実体を返さないと、どちらか片方のモックしか観測できない。
  const client = {
    finalGoal: {
      findMany: vi.fn(),
      findFirst: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      updateMany: vi.fn(),
      delete: vi.fn(),
    },
    $transaction: vi.fn(),
  };
  return { prisma: client, default: client };
});

const { auth } = await import("../auth.ts");
const { prisma } = await import("@/api/infra/prisma");
const { Prisma } = await import("@/api/generated/prisma/client");
const { registerGoalRoutes } = await import("./goals.ts");
const { buildTestApp, request, loggedInSession, demoSession } = await import(
  "../test-support.ts"
);

const getSession = auth.api.getSession as unknown as Mock;
const findMany = prisma.finalGoal.findMany as unknown as Mock;
const findFirst = prisma.finalGoal.findFirst as unknown as Mock;
const create = prisma.finalGoal.create as unknown as Mock;
const update = prisma.finalGoal.update as unknown as Mock;
const updateMany = prisma.finalGoal.updateMany as unknown as Mock;
const remove = prisma.finalGoal.delete as unknown as Mock;
const transaction = prisma.$transaction as unknown as Mock;

const app = buildTestApp(registerGoalRoutes);

const get = () => request(app, "GET", "/api/goals");
const post = (body: unknown) => request(app, "POST", "/api/goals", body);
const put = (body: unknown) => request(app, "PUT", "/api/goals/5", body);
const patch = (body: unknown) => request(app, "PATCH", "/api/goals/5", body);
const del = () => request(app, "DELETE", "/api/goals/5");

const ownedGoal = { id: 5, userId: "user-1", facultyId: 10 };

beforeEach(() => {
  vi.clearAllMocks();
  transaction.mockResolvedValue([]);
});

describe("GET /api/goals", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await get();

    expect(res.statusCode).toBe(401);
    expect(findMany).not.toHaveBeenCalled();
  });

  it("ログイン済みなら自分の志望校を 200 で返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const goals = [{ id: 1, facultyId: 10, userId: "user-1" }];
    findMany.mockResolvedValue(goals);

    const res = await get();

    expect(res.statusCode).toBe(200);
    expect(res.json()).toEqual(goals);
    // 自分のデータだけを取りに行っている
    expect(findMany).toHaveBeenCalledWith(
      expect.objectContaining({ where: { userId: "user-1" } })
    );
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
    // 未指定のステータスは decided になる
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          facultyId: 10,
          userId: "user-1",
          status: "decided",
        }),
      })
    );
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

describe("PUT /api/goals/:id", () => {
  it("他人の志望校は 404 を返す（更新しない）", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findFirst.mockResolvedValue(null);

    const res = await put({ facultyId: 20 });

    expect(res.statusCode).toBe(404);
    expect(update).not.toHaveBeenCalled();
    // 所有者チェックは where に userId を含めて1回のクエリで行う
    expect(findFirst).toHaveBeenCalledWith({
      where: { id: 5, userId: "user-1" },
    });
  });

  it("自分の志望校なら学部を差し替える", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findFirst.mockResolvedValue(ownedGoal);
    update.mockResolvedValue({ ...ownedGoal, facultyId: 20 });

    const res = await put({ facultyId: 20 });

    expect(res.statusCode).toBe(200);
    expect(update).toHaveBeenCalledWith({
      where: { id: 5 },
      data: { facultyId: 20 },
    });
  });
});

describe("PATCH /api/goals/:id", () => {
  it("デモアカウントなら 403 を返す", async () => {
    getSession.mockResolvedValue(demoSession);

    const res = await patch({ note: "メモ" });

    expect(res.statusCode).toBe(403);
    expect(findFirst).not.toHaveBeenCalled();
  });

  it("他人の志望校は 404 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findFirst.mockResolvedValue(null);

    const res = await patch({ note: "メモ" });

    expect(res.statusCode).toBe(404);
    expect(update).not.toHaveBeenCalled();
  });

  it("第一志望に立てるとき、既存の第一志望を同じトランザクションで外す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findFirst.mockResolvedValue(ownedGoal);

    const res = await patch({ isFirstChoice: true });

    expect(res.statusCode).toBe(200);
    // 第一志望は1ユーザー1校。外す→立てるを1つのトランザクションで行う
    expect(transaction).toHaveBeenCalledTimes(1);
    expect(updateMany).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      data: { isFirstChoice: false },
    });
    expect(update).toHaveBeenCalledWith({
      where: { id: 5 },
      data: { isFirstChoice: true },
    });
  });

  it("第一志望を外すだけならトランザクションは使わない", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findFirst.mockResolvedValue(ownedGoal);

    const res = await patch({ isFirstChoice: false });

    expect(res.statusCode).toBe(200);
    expect(transaction).not.toHaveBeenCalled();
    expect(update).toHaveBeenCalledWith({
      where: { id: 5 },
      data: { isFirstChoice: false },
    });
  });

  it("送られてきた項目だけを更新する（未指定は触らない）", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findFirst.mockResolvedValue(ownedGoal);

    const res = await patch({ note: "第一志望にしたい" });

    expect(res.statusCode).toBe(200);
    expect(update).toHaveBeenCalledTimes(1);
    expect(update).toHaveBeenCalledWith({
      where: { id: 5 },
      data: { note: "第一志望にしたい" },
    });
  });

  it("500文字を超えるメモは 400 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await patch({ note: "あ".repeat(501) });

    expect(res.statusCode).toBe(400);
    expect(findFirst).not.toHaveBeenCalled();
  });
});

describe("DELETE /api/goals/:id", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await del();

    expect(res.statusCode).toBe(401);
    expect(remove).not.toHaveBeenCalled();
  });

  it("デモアカウントなら 403 を返す（削除しない）", async () => {
    getSession.mockResolvedValue(demoSession);

    const res = await del();

    expect(res.statusCode).toBe(403);
    expect(remove).not.toHaveBeenCalled();
  });

  it("他人の志望校は 404 を返す（削除しない）", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findFirst.mockResolvedValue(null);

    const res = await del();

    expect(res.statusCode).toBe(404);
    expect(remove).not.toHaveBeenCalled();
  });

  it("自分の志望校なら削除する", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findFirst.mockResolvedValue(ownedGoal);
    remove.mockResolvedValue(ownedGoal);

    const res = await del();

    expect(res.statusCode).toBe(200);
    expect(remove).toHaveBeenCalledWith({ where: { id: 5 } });
  });
});
