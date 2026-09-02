import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { GET } from "./route";
import { auth } from "@/lib/auth";
import { pushLineText } from "@/lib/line";
import prisma from "@/lib/prisma";
import { exchangeLineLoginCode, getLineFriendshipStatus, verifyLineIdToken } from "@/lib/lineLogin";

vi.mock("@/lib/auth", () => ({ auth: { api: { getSession: vi.fn() } } }));
vi.mock("@/lib/line", () => ({ pushLineText: vi.fn() }));
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
  vi.mocked(pushLineText).mockResolvedValue(undefined);
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
    expect(pushLineText).toHaveBeenCalledWith(
      "U123",
      "受験マップとのLINE連携が完了しました！\n\n朝・夜の通知は、受験マップのプロフィールから設定できます。\nhttps://juken-map.com/line/settings",
      expect.any(AbortSignal)
    );
  });

  it("確認メッセージの送信に失敗しても連携は成功扱いにする", async () => {
    const transactionPrisma = {
      lineConnection: { findUnique: vi.fn().mockResolvedValue(null), upsert: vi.fn().mockResolvedValue({}) },
    };
    vi.mocked(prisma.$transaction).mockImplementation(async (callback) => callback(transactionPrisma as never));
    vi.mocked(pushLineText).mockRejectedValue(new Error("LINE API unavailable"));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    const response = await GET(new Request("https://juken-map.com/api/line/oauth/callback?code=code&state=state-1"));

    expect(response.headers.get("location")).toBe("https://juken-map.com/profile?line=connected#line-connection");
    expect(transactionPrisma.lineConnection.upsert).toHaveBeenCalled();
    expect(consoleError).toHaveBeenCalledWith(
      "[line-oauth] LINE connection completed, but confirmation message failed.",
      expect.any(Error)
    );
    consoleError.mockRestore();
  });

  it("確認メッセージが3秒以内に完了しなくても連携成功画面へ戻す", async () => {
    vi.useFakeTimers();
    const transactionPrisma = {
      lineConnection: { findUnique: vi.fn().mockResolvedValue(null), upsert: vi.fn().mockResolvedValue({}) },
    };
    vi.mocked(prisma.$transaction).mockImplementation(async (callback) => callback(transactionPrisma as never));
    let markPushStarted: () => void = () => {};
    const pushStarted = new Promise<void>((resolve) => { markPushStarted = resolve; });
    vi.mocked(pushLineText).mockImplementation((_lineUserId, _text, signal) => new Promise((_, reject) => {
      markPushStarted();
      signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")));
    }));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    try {
      const responsePromise = GET(new Request("https://juken-map.com/api/line/oauth/callback?code=code&state=state-1"));
      await pushStarted;
      await vi.advanceTimersByTimeAsync(3_000);
      const response = await responsePromise;

      expect(response.headers.get("location")).toBe("https://juken-map.com/profile?line=connected#line-connection");
      expect(consoleError).toHaveBeenCalledWith(
        "[line-oauth] LINE connection completed, but confirmation message failed.",
        expect.objectContaining({ name: "AbortError" })
      );
    } finally {
      consoleError.mockRestore();
      vi.useRealTimers();
    }
  });

  it("公式アカウントが友だちでなければ連携しない", async () => {
    vi.mocked(getLineFriendshipStatus).mockResolvedValue({ friendFlag: false });
    const response = await GET(new Request("https://juken-map.com/api/line/oauth/callback?code=code&state=state-1"));
    expect(response.headers.get("location")).toBe("https://juken-map.com/profile?line=friend-required#line-connection");
    expect(prisma.$transaction).not.toHaveBeenCalled();
    expect(pushLineText).not.toHaveBeenCalled();
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
