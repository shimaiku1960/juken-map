import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";
import { GET, POST } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { Prisma } from "@/app/generated/prisma/client";

// 外部境界だけを差し替える（DB・認証・リクエストヘッダー）。門番ロジック本体は実物を動かす。
vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    finalGoal: { findMany: vi.fn(), create: vi.fn() },
  },
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

// 型はゆるいモックとして扱う（戻り値・例外をテストごとに差し込む）
const getSession = auth.api.getSession as unknown as Mock;
const findMany = prisma.finalGoal.findMany as unknown as Mock;
const create = prisma.finalGoal.create as unknown as Mock;

// ログイン済みセッション（route が参照するのは user.id のみ）
const loggedInSession = { user: { id: "user-1" } };

// POST 用のリクエスト（実 HTTP は不要、json() だけあれば足りる）
const makeRequest = (body: unknown) =>
  ({ json: async () => body } as unknown as Request);

beforeEach(() => {
  vi.clearAllMocks();
});

describe("GET /api/goals", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await GET();

    expect(res.status).toBe(401);
    expect(findMany).not.toHaveBeenCalled();
  });

  it("ログイン済みなら自分の志望校を 200 で返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const goals = [{ id: 1, facultyId: 10, userId: "user-1" }];
    findMany.mockResolvedValue(goals);

    const res = await GET();

    expect(res.status).toBe(200);
    await expect(res.json()).resolves.toEqual(goals);
    // 自分のデータだけを取りに行っている
    expect(findMany).toHaveBeenCalledWith(
      expect.objectContaining({ where: { userId: "user-1" } })
    );
  });
});

describe("POST /api/goals", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await POST(makeRequest({ facultyId: 1 }));

    expect(res.status).toBe(401);
    expect(create).not.toHaveBeenCalled();
  });

  it("デモアカウントなら 403 を返す（作成しない）", async () => {
    getSession.mockResolvedValue({
      user: { id: "demo-1", email: "demo@juken-map.com" },
    });

    const res = await POST(makeRequest({ facultyId: 10 }));

    expect(res.status).toBe(403);
    expect(create).not.toHaveBeenCalled();
  });

  it("入力が不正なら 400 を返す（Zod）", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await POST(makeRequest({})); // facultyId が無い

    expect(res.status).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("正常なら 201 で作成した志望校を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const created = { id: 5, facultyId: 10, userId: "user-1" };
    create.mockResolvedValue(created);

    const res = await POST(makeRequest({ facultyId: 10 }));

    expect(res.status).toBe(201);
    await expect(res.json()).resolves.toEqual(created);
  });

  it("重複登録（P2002）は 409 に変換する", async () => {
    getSession.mockResolvedValue(loggedInSession);
    create.mockRejectedValue(
      new Prisma.PrismaClientKnownRequestError("Unique constraint failed", {
        code: "P2002",
        clientVersion: "7.8.0",
      })
    );

    const res = await POST(makeRequest({ facultyId: 10 }));

    expect(res.status).toBe(409);
  });

  it("想定外のエラーはそのまま投げる（握りつぶさない）", async () => {
    getSession.mockResolvedValue(loggedInSession);
    create.mockRejectedValue(new Error("DB down"));

    await expect(POST(makeRequest({ facultyId: 10 }))).rejects.toThrow(
      "DB down"
    );
  });
});
