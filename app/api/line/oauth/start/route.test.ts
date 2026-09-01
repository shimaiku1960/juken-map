import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { GET } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { createLineOAuthValues, lineLoginAuthorizationUrl } from "@/lib/lineLogin";

vi.mock("@/lib/auth", () => ({ auth: { api: { getSession: vi.fn() } } }));
vi.mock("@/lib/prisma", () => ({
  default: {
    $transaction: vi.fn(),
    lineOAuthAttempt: { create: vi.fn(), deleteMany: vi.fn() },
  },
}));
vi.mock("@/lib/lineLogin", () => ({
  createLineOAuthValues: vi.fn(),
  lineLoginAuthorizationUrl: vi.fn(),
}));
vi.mock("next/headers", () => ({ headers: vi.fn(() => new Headers()) }));

const getSession = auth.api.getSession as unknown as Mock;

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(createLineOAuthValues).mockReturnValue({
    state: "state-1",
    nonce: "nonce-1",
    codeVerifier: "verifier-1",
    codeChallenge: "challenge-1",
  });
  vi.mocked(lineLoginAuthorizationUrl).mockReturnValue(new URL("https://access.line.me/oauth2/v2.1/authorize?state=state-1"));
  vi.mocked(prisma.$transaction).mockResolvedValue([]);
});

describe("GET /api/line/oauth/start", () => {
  it("未ログインなら通知設定へ戻るログイン導線へ送る", async () => {
    getSession.mockResolvedValue(null);
    const response = await GET(new Request("http://localhost:3000/api/line/oauth/start"));
    expect(response.headers.get("location")).toContain("/login?callbackURL=");
    expect(prisma.$transaction).not.toHaveBeenCalled();
  });

  it("認証済みユーザーに10分間の単回OAuth試行を作る", async () => {
    getSession.mockResolvedValue({ user: { id: "user-1" } });
    const response = await GET(new Request("http://localhost:3000/api/line/oauth/start"));

    expect(response.headers.get("location")).toContain("access.line.me/oauth2/v2.1/authorize");
    expect(prisma.lineOAuthAttempt.deleteMany).toHaveBeenCalledWith({ where: { userId: "user-1" } });
    expect(prisma.lineOAuthAttempt.create).toHaveBeenCalledWith({
      data: expect.objectContaining({
        state: "state-1",
        nonce: "nonce-1",
        codeVerifier: "verifier-1",
        userId: "user-1",
        expiresAt: expect.any(Date),
      }),
    });
  });
});
