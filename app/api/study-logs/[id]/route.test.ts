import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { DELETE, PATCH } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    studyLog: { findUnique: vi.fn(), update: vi.fn(), delete: vi.fn() },
    textbook: { findFirst: vi.fn() },
  },
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findUnique = prisma.studyLog.findUnique as unknown as Mock;
const del = prisma.studyLog.delete as unknown as Mock;
const update = prisma.studyLog.update as unknown as Mock;
const findTextbook = prisma.textbook.findFirst as unknown as Mock;

const loggedInSession = { user: { id: "user-1", email: "me@example.com" } };

const makeParams = (id: string) => ({ params: Promise.resolve({ id }) });
const makeRequest = (body: unknown) =>
  ({ json: async () => body } as unknown as Request);
const validBody = { date: "2026-02-20", minutes: 90, subject: "english" };

beforeEach(() => {
  vi.clearAllMocks();
});

describe("PATCH /api/study-logs/[id]", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await PATCH(makeRequest(validBody), makeParams("1"));

    expect(res.status).toBe(401);
    expect(update).not.toHaveBeenCalled();
  });

  it("他人の実績は 404 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findUnique.mockResolvedValue({ id: 1, userId: "someone-else" });

    const res = await PATCH(makeRequest(validBody), makeParams("1"));

    expect(res.status).toBe(404);
    expect(update).not.toHaveBeenCalled();
  });

  it("他人の参考書IDなら 400 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findUnique.mockResolvedValue({ id: 1, userId: "user-1" });
    findTextbook.mockResolvedValue(null);

    const res = await PATCH(
      makeRequest({ ...validBody, textbookId: 999 }),
      makeParams("1")
    );

    expect(res.status).toBe(400);
    expect(update).not.toHaveBeenCalled();
  });

  it("自分の実績なら入力内容を更新する", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findUnique.mockResolvedValue({ id: 1, userId: "user-1" });
    update.mockResolvedValue({ id: 1, ...validBody });

    const res = await PATCH(makeRequest(validBody), makeParams("1"));

    expect(res.status).toBe(200);
    expect(update).toHaveBeenCalledWith({
      where: { id: 1 },
      data: expect.objectContaining({
        minutes: 90,
        subject: "english",
      }),
    });
  });

  it("予定由来の実績は日付・科目・参考書の紐づきを維持する", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findUnique.mockResolvedValue({
      id: 1,
      userId: "user-1",
      studyPlanId: 8,
      date: new Date("2026-02-20"),
      subject: "math",
      textbookId: 3,
    });
    findTextbook.mockResolvedValue({
      id: 9,
      userId: "user-1",
      rangeUnit: null,
      totalAmount: null,
    });
    update.mockResolvedValue({ id: 1 });

    const res = await PATCH(
      makeRequest({
        ...validBody,
        date: "2026-02-21",
        subject: "english",
        textbookId: 9,
      }),
      makeParams("1")
    );

    expect(res.status).toBe(200);
    expect(update).toHaveBeenCalledWith({
      where: { id: 1 },
      data: expect.objectContaining({
        date: new Date("2026-02-20"),
        subject: "math",
        textbookId: 3,
      }),
    });
  });

  it("時間だけの修正では現在の参考書設定を過去実績へ遡及しない", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findUnique.mockResolvedValue({
      id: 1,
      userId: "user-1",
      studyPlanId: null,
      date: new Date("2026-02-20"),
      minutes: 60,
      subject: "english",
      textbookId: 3,
      rangeStart: 1,
      rangeEnd: 10,
      rangeUnit: "page",
    });
    update.mockResolvedValue({ id: 1 });

    const res = await PATCH(
      makeRequest({
        date: "2026-02-20",
        minutes: 90,
        subject: "english",
        textbookId: 3,
        rangeStart: 1,
        rangeEnd: 10,
        rangeUnit: "page",
      }),
      makeParams("1")
    );

    expect(res.status).toBe(200);
    expect(findTextbook).not.toHaveBeenCalled();
    expect(update).toHaveBeenCalled();
  });
});

describe("DELETE /api/study-logs/[id]", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await DELETE({} as Request, makeParams("1"));

    expect(res.status).toBe(401);
    expect(del).not.toHaveBeenCalled();
  });

  it("デモアカウントなら 403 を返す（削除しない）", async () => {
    getSession.mockResolvedValue({
      user: { id: "demo-1", email: "demo@juken-map.com" },
    });

    const res = await DELETE({} as Request, makeParams("1"));

    expect(res.status).toBe(403);
    expect(del).not.toHaveBeenCalled();
  });

  it("他人の実績は 404 を返す（削除しない）", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findUnique.mockResolvedValue({ id: 1, userId: "someone-else" });

    const res = await DELETE({} as Request, makeParams("1"));

    expect(res.status).toBe(404);
    expect(del).not.toHaveBeenCalled();
  });

  it("存在しない実績は 404 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findUnique.mockResolvedValue(null);

    const res = await DELETE({} as Request, makeParams("999"));

    expect(res.status).toBe(404);
    expect(del).not.toHaveBeenCalled();
  });

  it("自分の実績なら 200 で削除する", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findUnique.mockResolvedValue({ id: 1, userId: "user-1" });
    del.mockResolvedValue({ id: 1 });

    const res = await DELETE({} as Request, makeParams("1"));

    expect(res.status).toBe(200);
    expect(del).toHaveBeenCalledWith({ where: { id: 1 } });
  });
});
