import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { POST } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/demo", () => ({
  demoReadOnlyGuard: vi.fn(() => null),
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    supportSubscription: { findUnique: vi.fn() },
    supportLineConnection: { findUnique: vi.fn(), upsert: vi.fn() },
  },
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findSubscription = prisma.supportSubscription.findUnique as unknown as Mock;
const findConnection = prisma.supportLineConnection.findUnique as unknown as Mock;
const upsertConnection = prisma.supportLineConnection.upsert as unknown as Mock;

beforeEach(() => {
  vi.clearAllMocks();
  getSession.mockResolvedValue({ user: { id: "user-1" } });
  findSubscription.mockResolvedValue({ status: "trialing" });
  findConnection.mockResolvedValue(null);
  upsertConnection.mockResolvedValue({ id: "connection-1" });
});

describe("POST /api/support/line-code", () => {
  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    const response = await POST();

    expect(response.status).toBe(401);
    expect(upsertConnection).not.toHaveBeenCalled();
  });

  it("有効な契約がなければ403を返す", async () => {
    findSubscription.mockResolvedValue({ status: "canceled" });

    const response = await POST();

    expect(response.status).toBe(403);
    expect(upsertConnection).not.toHaveBeenCalled();
  });

  it("連携済みなら409を返す", async () => {
    findConnection.mockResolvedValue({ linkedAt: new Date() });

    const response = await POST();

    expect(response.status).toBe(409);
    expect(upsertConnection).not.toHaveBeenCalled();
  });

  it("期限付きコードを発行しハッシュだけ保存する", async () => {
    const response = await POST();
    const body = (await response.json()) as { code: string; expiresAt: string };

    expect(response.status).toBe(200);
    expect(body.code).toMatch(/^[A-Z0-9_-]{10}$/);
    expect(new Date(body.expiresAt).getTime()).toBeGreaterThan(Date.now());
    expect(upsertConnection).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      create: expect.objectContaining({
        userId: "user-1",
        codeHash: expect.stringMatching(/^[a-f0-9]{64}$/),
      }),
      update: expect.objectContaining({
        codeHash: expect.stringMatching(/^[a-f0-9]{64}$/),
        lineDisplayName: null,
      }),
    });
    expect(JSON.stringify(upsertConnection.mock.calls)).not.toContain(body.code);
  });
});
