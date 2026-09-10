import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/api/infra/prisma", () => ({
  prisma: {
    user: { updateMany: vi.fn() },
    studyPlan: { findUnique: vi.fn(), findFirst: vi.fn(), update: vi.fn() },
    studyLog: { count: vi.fn(), create: vi.fn() },
    textbook: { count: vi.fn() },
    $transaction: vi.fn(),
  },
}));

vi.mock("@/api/services/study-plan-service", () => ({
  listStudyPlans: vi.fn(),
}));

const { auth } = await import("../auth.ts");
const { prisma } = await import("@/api/infra/prisma");
const { Prisma } = await import("@/api/generated/prisma/client");
const { registerStudyPlanRoutes } = await import("./study-plans.ts");
const { buildTestApp, request, loggedInSession, demoSession } = await import(
  "../test-support.ts"
);

const getSession = auth.api.getSession as unknown as Mock;
const findPlan = prisma.studyPlan.findUnique as unknown as Mock;
const findPlanFirst = prisma.studyPlan.findFirst as unknown as Mock;
const updatePlan = prisma.studyPlan.update as unknown as Mock;
const countLogs = prisma.studyLog.count as unknown as Mock;
const createLog = prisma.studyLog.create as unknown as Mock;
const markActivation = prisma.user.updateMany as unknown as Mock;
const transaction = prisma.$transaction as unknown as Mock;

const app = buildTestApp(registerStudyPlanRoutes);

const patch = (body: unknown) => request(app, "PATCH", "/api/study-plans/7", body);
const complete = (body: unknown) =>
  request(app, "POST", "/api/study-plans/7/complete", body);

describe("PATCH /api/study-plans/:id", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getSession.mockResolvedValue(loggedInSession);
    findPlan.mockResolvedValue({ id: 7, userId: "user-1" });
    updatePlan.mockResolvedValue({ id: 7, done: false });
  });

  it("実績記録済みの予定は未完了に戻せない", async () => {
    countLogs.mockResolvedValue(1);

    const res = await patch({ done: false });

    expect(res.statusCode).toBe(409);
    expect(countLogs).toHaveBeenCalledWith({ where: { studyPlanId: 7 } });
    expect(updatePlan).not.toHaveBeenCalled();
  });

  it("実績がなければ予定のみ未完了へ戻せる", async () => {
    countLogs.mockResolvedValue(0);

    const res = await patch({ done: false });

    expect(res.statusCode).toBe(200);
    expect(updatePlan).toHaveBeenCalledWith(
      expect.objectContaining({ data: expect.objectContaining({ done: false }) })
    );
  });
});

describe("POST /api/study-plans/:id/complete", () => {
  const plan = {
    id: 7,
    userId: "user-1",
    date: new Date("2027-02-20"),
    subject: "english",
    textbookId: 3,
    textbook: { id: 3, totalAmount: 300, rangeUnit: "page" },
    rangeStart: 10,
    rangeEnd: 20,
    rangeUnit: "page",
    studyLog: null,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    getSession.mockResolvedValue(loggedInSession);
    findPlanFirst.mockResolvedValue(plan);
    markActivation.mockResolvedValue({ count: 0 });
    createLog.mockResolvedValue({ id: 11 });
    updatePlan.mockResolvedValue({ ...plan, done: true });
    transaction.mockImplementation((callback: (tx: unknown) => unknown) =>
      callback(prisma)
    );
  });

  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await complete({ minutes: 30 });

    expect(res.statusCode).toBe(401);
    expect(findPlanFirst).not.toHaveBeenCalled();
  });

  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue(demoSession);

    const res = await complete({ minutes: 30 });

    expect(res.statusCode).toBe(403);
    expect(findPlanFirst).not.toHaveBeenCalled();
  });

  it("学習時間が不正なら400を返す", async () => {
    const res = await complete({ minutes: 0 });

    expect(res.statusCode).toBe(400);
    expect(findPlanFirst).not.toHaveBeenCalled();
  });

  it("他人の予定なら404を返す", async () => {
    findPlanFirst.mockResolvedValue(null);

    const res = await complete({ minutes: 30 });

    expect(res.statusCode).toBe(404);
    expect(findPlanFirst).toHaveBeenCalledWith(
      expect.objectContaining({ where: { id: 7, userId: "user-1" } })
    );
  });

  it("実績を記録済みなら409を返す", async () => {
    findPlanFirst.mockResolvedValue({ ...plan, studyLog: { id: 11 } });

    const res = await complete({ minutes: 30 });

    expect(res.statusCode).toBe(409);
    expect(transaction).not.toHaveBeenCalled();
  });

  it("実績作成と予定完了を同じtransactionで処理する", async () => {
    const res = await complete({
      minutes: 45,
      rangeStart: 12,
      rangeEnd: 18,
      rangeUnit: "page",
      memo: "復習した",
    });

    expect(res.statusCode).toBe(201);
    expect(createLog).toHaveBeenCalledWith({
      data: expect.objectContaining({
        userId: "user-1",
        studyPlanId: 7,
        date: plan.date,
        minutes: 45,
        subject: "english",
        textbookId: 3,
        rangeStart: 12,
        rangeEnd: 18,
        rangeUnit: "page",
        memo: "復習した",
      }),
      include: { textbook: true },
    });
    expect(updatePlan).toHaveBeenCalledWith({
      where: { id: 7 },
      data: { done: true },
    });
    expect(transaction).toHaveBeenCalledTimes(1);
  });

  it("同時完了による重複作成（P2002）は409に変換する", async () => {
    transaction.mockRejectedValue(
      new Prisma.PrismaClientKnownRequestError("Unique constraint failed", {
        code: "P2002",
        clientVersion: "7.9.1",
      })
    );

    const res = await complete({ minutes: 30 });

    expect(res.statusCode).toBe(409);
    expect(res.json()).toEqual({
      error: "この予定の実績はすでに記録されています",
    });
  });

  it("想定外のtransactionエラーは握りつぶさず500になる", async () => {
    transaction.mockRejectedValue(new Error("DB down"));

    const res = await complete({ minutes: 30 });

    expect(res.statusCode).toBe(500);
  });
});
