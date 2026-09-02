import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { GET } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { exchangeLineLoginCode, getLineFriendshipStatus, verifyLineIdToken } from "@/lib/lineLogin";

vi.mock("@/lib/auth", () => ({ auth: { api: { getSession: vi.fn() } } }));
vi.mock("@/lib/prisma", () => ({
  default: {
    $transaction: vi.fn(),
    lineOAuthAttempt: { findUnique: vi.fn(), delete: vi.fn(), deleteMany: vi.fn() },
    lineConnection: { findUnique: vi.fn(), upsert: vi.fn() },
  },
}));
vi.mock("@/lib/lineLogin", () => ({
  exchangeLineLoginCode: vi.fn(),
  getLineFriendshipStatus: vi.fn(),
  verifyLineIdToken: vi.fn(),
}));
vi.mock("next/headers", () => ({ headers: vi.fn(() => new Headers()) }));

const getSession = auth.api.getSession as unknown as Mock;
const attempt = {
  state: "state-1",
  userId: "user-1",
  nonce: "nonce-1",
  codeVerifier: "verifier",
  redirectUri: "https://juken-map.com/api/line/oauth/callback",
  expiresAt: new Date(Date.now() + 60_000),
  createdAt: new Date(),
};

beforeEach(() => {
  vi.clearAllMocks();
  getSession.mockResolvedValue({ user: { id: "user-1" } });
  vi.mocked(prisma.lineOAuthAttempt.findUnique).mockResolvedValue(attempt);
  vi.mocked(prisma.lineOAuthAttempt.delete).mockResolvedValue(attempt);
  vi.mocked(exchangeLineLoginCode).mockResolvedValue({ access_token: "access", id_token: "id-token" });
  vi.mocked(verifyLineIdToken).mockResolvedValue({ sub: "U123", nonce: "nonce-1" });
  vi.mocked(getLineFriendshipStatus).mockResolvedValue({ friendFlag: true });
});

describe("GET /api/line/oauth/callback", () => {
  it("友だち状態とID tokenを確認して連携する", async () => {
    const transactionPrisma = {
      lineConnection: { findUnique: vi.fn().mockResolvedValue(null), upsert: vi.fn().mockResolvedValue({}) },
    };
    vi.mocked(prisma.$transaction).mockImplementation(async (callback) => callback(transactionPrisma as never));

    const response = await GET(new Request("https://juken-map.com/api/line/oauth/callback?code=code&state=state-1"));

    expect(response.headers.get("location")).toBe("https://juken-map.com/profile?line=connected#line-connection");
    expect(prisma.lineOAuthAttempt.delete).toHaveBeenCalledWith({ where: { state: "state-1" } });
    expect(transactionPrisma.lineConnection.upsert).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      create: { userId: "user-1", lineUserId: "U123" },
      update: { lineUserId: "U123", linkedAt: expect.any(Date) },
    });
  });

  it("公式アカウントが友だちでなければ連携しない", async () => {
    vi.mocked(getLineFriendshipStatus).mockResolvedValue({ friendFlag: false });
    const response = await GET(new Request("https://juken-map.com/api/line/oauth/callback?code=code&state=state-1"));
    expect(response.headers.get("location")).toBe("https://juken-map.com/profile?line=friend-required#line-connection");
    expect(prisma.$transaction).not.toHaveBeenCalled();
  });

  it("別ユーザーのstateは利用できない", async () => {
    vi.mocked(prisma.lineOAuthAttempt.findUnique).mockResolvedValue({ ...attempt, userId: "other-user" });
    const response = await GET(new Request("https://juken-map.com/api/line/oauth/callback?code=code&state=state-1"));
    expect(response.headers.get("location")).toBe("https://juken-map.com/profile?line=expired#line-connection");
    expect(exchangeLineLoginCode).not.toHaveBeenCalled();
  });

  it("セッション切れならcallbackURLを保持してログインへ送る", async () => {
    getSession.mockResolvedValue(null);
    const response = await GET(new Request("https://juken-map.com/api/line/oauth/callback?code=code&state=state-1"));
    expect(response.headers.get("location")).toContain("/login?callbackURL=");
    expect(prisma.lineOAuthAttempt.findUnique).not.toHaveBeenCalled();
  });
});
