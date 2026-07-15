import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { PATCH } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    studyPlan: { findUnique: vi.fn(), update: vi.fn() },
    studyLog: { count: vi.fn() },
    textbook: { count: vi.fn() },
  },
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findPlan = prisma.studyPlan.findUnique as unknown as Mock;
const updatePlan = prisma.studyPlan.update as unknown as Mock;
const countLogs = prisma.studyLog.count as unknown as Mock;

const request = (body: unknown) =>
  ({ json: async () => body } as unknown as Request);
const params = { params: Promise.resolve({ id: "7" }) };

beforeEach(() => {
  vi.clearAllMocks();
  getSession.mockResolvedValue({
    user: { id: "user-1", email: "me@example.com" },
  });
  findPlan.mockResolvedValue({ id: 7, userId: "user-1" });
  updatePlan.mockResolvedValue({ id: 7, done: false });
});

describe("PATCH /api/study-plans/[id]", () => {
  it("実績記録済みの予定は未完了に戻せない", async () => {
    countLogs.mockResolvedValue(1);

    const response = await PATCH(request({ done: false }), params);

    expect(response.status).toBe(409);
    expect(countLogs).toHaveBeenCalledWith({ where: { studyPlanId: 7 } });
    expect(updatePlan).not.toHaveBeenCalled();
  });

  it("実績がなければ予定のみ未完了へ戻せる", async () => {
    countLogs.mockResolvedValue(0);

    const response = await PATCH(request({ done: false }), params);

    expect(response.status).toBe(200);
    expect(updatePlan).toHaveBeenCalledWith(
      expect.objectContaining({ data: expect.objectContaining({ done: false }) })
    );
  });
});
