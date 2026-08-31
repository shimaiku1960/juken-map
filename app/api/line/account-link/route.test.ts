import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { POST } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

vi.mock("@/lib/auth", () => ({ auth: { api: { getSession: vi.fn() } } }));
vi.mock("@/lib/prisma", () => ({
  default: {
    $transaction: vi.fn(),
    lineLinkNonce: { create: vi.fn(), deleteMany: vi.fn() },
  },
}));
vi.mock("next/headers", () => ({ headers: vi.fn(() => new Headers()) }));

const getSession = auth.api.getSession as unknown as Mock;
const request = (body: unknown) => ({ json: async () => body } as Request);

beforeEach(() => vi.clearAllMocks());

describe("POST /api/line/account-link", () => {
  it("未ログインならnonceを発行しない", async () => {
    getSession.mockResolvedValue(null);
    expect((await POST(request({ linkToken: "token" }))).status).toBe(401);
    expect(prisma.lineLinkNonce.create).not.toHaveBeenCalled();
  });

  it("ログインユーザーに10分間の単回nonceを発行する", async () => {
    getSession.mockResolvedValue({ user: { id: "user-1" } });
    vi.mocked(prisma.lineLinkNonce.create).mockResolvedValue({} as never);
    vi.mocked(prisma.lineLinkNonce.deleteMany).mockResolvedValue({ count: 0 });
    vi.mocked(prisma.$transaction).mockResolvedValue([]);
    const before = Date.now();
    const response = await POST(request({ linkToken: "link-token" }));
    const result = await response.json();

    expect(response.status).toBe(200);
    expect(result.redirectUrl).toContain("https://access.line.me/dialog/bot/accountLink");
    expect(prisma.lineLinkNonce.create).toHaveBeenCalledWith({
      data: expect.objectContaining({
        userId: "user-1",
        nonce: expect.any(String),
        expiresAt: expect.any(Date),
      }),
    });
    expect(prisma.lineLinkNonce.deleteMany).toHaveBeenCalledWith({
      where: { userId: "user-1" },
    });
    const expiresAt = vi.mocked(prisma.lineLinkNonce.create).mock.calls[0][0].data.expiresAt;
    expect(new Date(expiresAt).getTime()).toBeGreaterThanOrEqual(before + 9 * 60 * 1000);
  });
});
