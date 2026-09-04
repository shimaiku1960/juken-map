import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { POST } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { Prisma } from "@/app/generated/prisma/client";

vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    user: { updateMany: vi.fn() },
    studyPlan: { findFirst: vi.fn(), update: vi.fn() },
    studyLog: { create: vi.fn() },
    $transaction: vi.fn(),
  },
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findPlan = prisma.studyPlan.findFirst as unknown as Mock;
const createLog = prisma.studyLog.create as unknown as Mock;
const markActivation = prisma.user.updateMany as unknown as Mock;
const updatePlan = prisma.studyPlan.update as unknown as Mock;
const transaction = prisma.$transaction as unknown as Mock;

const session = { user: { id: "user-1", email: "me@example.com" } };
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

const request = (body: unknown) =>
  ({ json: async () => body } as unknown as Request);
const params = { params: Promise.resolve({ id: "7" }) };

beforeEach(() => {
  vi.clearAllMocks();
  getSession.mockResolvedValue(session);
  findPlan.mockResolvedValue(plan);
  markActivation.mockResolvedValue({ count: 0 });
  createLog.mockReturnValue(Promise.resolve({ id: 11 }));
  updatePlan.mockReturnValue(Promise.resolve({ ...plan, done: true }));
  transaction.mockImplementation((callback) => callback(prisma));
});

describe("POST /api/study-plans/[id]/complete", () => {
  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    const response = await POST(request({ minutes: 30 }), params);

    expect(response.status).toBe(401);
    expect(findPlan).not.toHaveBeenCalled();
  });

  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue({
      user: { id: "demo-1", email: "demo@juken-map.com" },
    });

    const response = await POST(request({ minutes: 30 }), params);

    expect(response.status).toBe(403);
    expect(findPlan).not.toHaveBeenCalled();
  });

  it("学習時間が不正なら400を返す", async () => {
    const response = await POST(request({ minutes: 0 }), params);

    expect(response.status).toBe(400);
    expect(findPlan).not.toHaveBeenCalled();
  });

  it("他人の予定なら404を返す", async () => {
    findPlan.mockResolvedValue(null);

    const response = await POST(request({ minutes: 30 }), params);

    expect(response.status).toBe(404);
    expect(findPlan).toHaveBeenCalledWith(
      expect.objectContaining({ where: { id: 7, userId: "user-1" } })
    );
  });

  it("実績を記録済みなら409を返す", async () => {
    findPlan.mockResolvedValue({ ...plan, studyLog: { id: 11 } });

    const response = await POST(request({ minutes: 30 }), params);

    expect(response.status).toBe(409);
    expect(transaction).not.toHaveBeenCalled();
  });

  it("実績作成と予定完了を同じtransactionで処理する", async () => {
    const response = await POST(
      request({
        minutes: 45,
        rangeStart: 12,
        rangeEnd: 18,
        rangeUnit: "page",
        memo: "復習した",
      }),
      params
    );

    expect(response.status).toBe(201);
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
        clientVersion: "7.8.0",
      })
    );

    const response = await POST(request({ minutes: 30 }), params);

    expect(response.status).toBe(409);
    await expect(response.json()).resolves.toEqual({
      error: "この予定の実績はすでに記録されています",
    });
  });

  it("想定外のtransactionエラーはそのまま投げる", async () => {
    transaction.mockRejectedValue(new Error("DB down"));

    await expect(POST(request({ minutes: 30 }), params)).rejects.toThrow(
      "DB down"
    );
  });
});
