import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { GET, POST } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

// 外部境界だけを差し替える（DB・認証・リクエストヘッダー）。門番ロジック本体は実物を動かす。
vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    studyLog: { findMany: vi.fn(), create: vi.fn() },
    textbook: { findFirst: vi.fn() },
  },
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findMany = prisma.studyLog.findMany as unknown as Mock;
const create = prisma.studyLog.create as unknown as Mock;
const findTextbook = prisma.textbook.findFirst as unknown as Mock;

const loggedInSession = { user: { id: "user-1", email: "me@example.com" } };

const makeRequest = (body: unknown) =>
  ({ json: async () => body } as unknown as Request);

const validBody = { date: "2026-02-20", minutes: 60, subject: "english" };

beforeEach(() => {
  vi.clearAllMocks();
});

describe("GET /api/study-logs", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await GET();

    expect(res.status).toBe(401);
    expect(findMany).not.toHaveBeenCalled();
  });

  it("ログイン済みなら自分の実績を 200 で返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const logs = [{ id: 1, minutes: 60, userId: "user-1" }];
    findMany.mockResolvedValue(logs);

    const res = await GET();

    expect(res.status).toBe(200);
    await expect(res.json()).resolves.toEqual(logs);
    expect(findMany).toHaveBeenCalledWith(
      expect.objectContaining({ where: { userId: "user-1" } })
    );
  });
});

describe("POST /api/study-logs", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await POST(makeRequest(validBody));

    expect(res.status).toBe(401);
    expect(create).not.toHaveBeenCalled();
  });

  it("デモアカウントなら 403 を返す（作成しない）", async () => {
    getSession.mockResolvedValue({
      user: { id: "demo-1", email: "demo@juken-map.com" },
    });

    const res = await POST(makeRequest(validBody));

    expect(res.status).toBe(403);
    expect(create).not.toHaveBeenCalled();
  });

  it("入力が不正なら 400 を返す（minutes 欠落）", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await POST(makeRequest({ date: "2026-02-20" }));

    expect(res.status).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("未来日の実績なら 400 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await POST(
      makeRequest({ ...validBody, date: "2999-01-01" })
    );

    expect(res.status).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("他人の参考書IDなら 400 を返す（所有チェック）", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findTextbook.mockResolvedValue(null); // 自分の所有分に無い

    const res = await POST(
      makeRequest({ ...validBody, textbookId: 999 })
    );

    expect(res.status).toBe(400);
    expect(create).not.toHaveBeenCalled();
    expect(findTextbook).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { id: 999, userId: "user-1" },
      })
    );
  });

  it("逆算設定と実績の単位が異なるなら 400 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findTextbook.mockResolvedValue({
      id: 3,
      userId: "user-1",
      totalAmount: 300,
      rangeUnit: "page",
    });

    const res = await POST(
      makeRequest({
        ...validBody,
        textbookId: 3,
        rangeStart: 1,
        rangeEnd: 10,
        rangeUnit: "question",
      })
    );

    expect(res.status).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("実績の終了位置が参考書の総量を超えたら 400 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findTextbook.mockResolvedValue({
      id: 3,
      userId: "user-1",
      totalAmount: 300,
      rangeUnit: "page",
    });

    const res = await POST(
      makeRequest({
        ...validBody,
        textbookId: 3,
        rangeStart: 290,
        rangeEnd: 301,
        rangeUnit: "page",
      })
    );

    expect(res.status).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("正常なら 201 で作成した実績を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const created = { id: 5, minutes: 60, userId: "user-1" };
    create.mockResolvedValue(created);

    const res = await POST(makeRequest(validBody));

    expect(res.status).toBe(201);
    await expect(res.json()).resolves.toEqual(created);
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ minutes: 60, userId: "user-1" }),
      })
    );
  });

  it("想定外のエラーはそのまま投げる（握りつぶさない）", async () => {
    getSession.mockResolvedValue(loggedInSession);
    create.mockRejectedValue(new Error("DB down"));

    await expect(POST(makeRequest(validBody))).rejects.toThrow("DB down");
  });
});
