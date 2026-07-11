import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { DELETE } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    studyLog: { findUnique: vi.fn(), delete: vi.fn() },
  },
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findUnique = prisma.studyLog.findUnique as unknown as Mock;
const del = prisma.studyLog.delete as unknown as Mock;

const loggedInSession = { user: { id: "user-1", email: "me@example.com" } };

const makeParams = (id: string) => ({ params: Promise.resolve({ id }) });

beforeEach(() => {
  vi.clearAllMocks();
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
