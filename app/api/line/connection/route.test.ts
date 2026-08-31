import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { DELETE } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

vi.mock("@/lib/auth", () => ({ auth: { api: { getSession: vi.fn() } } }));
vi.mock("@/lib/demo", () => ({ demoReadOnlyGuard: vi.fn(() => null) }));
vi.mock("@/lib/prisma", () => ({
  default: {
    $transaction: vi.fn(),
    notificationPreference: { updateMany: vi.fn() },
    lineConnection: { deleteMany: vi.fn() },
    lineLinkNonce: { deleteMany: vi.fn() },
  },
}));
vi.mock("next/headers", () => ({ headers: vi.fn(() => new Headers()) }));

const getSession = auth.api.getSession as unknown as Mock;

beforeEach(() => vi.clearAllMocks());

describe("DELETE /api/line/connection", () => {
  it("未ログインなら連携を解除しない", async () => {
    getSession.mockResolvedValue(null);
    expect((await DELETE()).status).toBe(401);
    expect(prisma.$transaction).not.toHaveBeenCalled();
  });

  it("LINE設定だけを無効にして連携情報を削除する", async () => {
    getSession.mockResolvedValue({ user: { id: "user-1" } });
    vi.mocked(prisma.notificationPreference.updateMany).mockResolvedValue({ count: 1 });
    vi.mocked(prisma.lineConnection.deleteMany).mockResolvedValue({ count: 1 });
    vi.mocked(prisma.lineLinkNonce.deleteMany).mockResolvedValue({ count: 0 });
    vi.mocked(prisma.$transaction).mockResolvedValue([]);

    const response = await DELETE();

    expect(response.status).toBe(200);
    expect(prisma.notificationPreference.updateMany).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      data: { lineMorningEnabled: false, lineEveningEnabled: false },
    });
    expect(prisma.$transaction).toHaveBeenCalledOnce();
  });
});
