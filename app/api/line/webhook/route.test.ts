import { beforeEach, describe, expect, it, vi } from "vitest";
import { POST } from "./route";
import { issueLineLinkToken, replyLineText, verifyLineSignature } from "@/lib/line";
import prisma from "@/lib/prisma";

vi.mock("@/lib/line", () => ({
  verifyLineSignature: vi.fn(),
  issueLineLinkToken: vi.fn(),
  lineAccountLinkUrl: vi.fn(() => "https://juken-map.com/line/link?linkToken=token"),
  replyLineText: vi.fn(),
}));
const transactionPrisma = {
  lineConnection: { findUnique: vi.fn(), upsert: vi.fn() },
  lineLinkNonce: { delete: vi.fn() },
};
vi.mock("@/lib/prisma", () => ({
  default: {
    $transaction: vi.fn((callback) => callback(transactionPrisma)),
    lineConnection: { findUnique: vi.fn() },
    lineLinkNonce: { findUnique: vi.fn() },
  },
}));

const request = (events: unknown[]) =>
  new Request("https://juken-map.com/api/line/webhook", {
    method: "POST",
    headers: { "x-line-signature": "signature" },
    body: JSON.stringify({ events }),
  });

beforeEach(() => vi.clearAllMocks());

describe("POST /api/line/webhook", () => {
  it("署名が不正なら401を返す", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(false);
    expect((await POST(request([]))).status).toBe(401);
  });

  it("連携メッセージへ公式Account Linking URLを返す", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(prisma.lineConnection.findUnique).mockResolvedValue(null);
    vi.mocked(issueLineLinkToken).mockResolvedValue("token");
    vi.mocked(replyLineText).mockResolvedValue(undefined);

    const response = await POST(request([{
      type: "message",
      replyToken: "reply-token",
      source: { type: "user", userId: "U123" },
      message: { type: "text", text: "連携" },
    }]));

    expect(response.status).toBe(200);
    expect(issueLineLinkToken).toHaveBeenCalledWith("U123");
    expect(replyLineText).toHaveBeenCalledWith("reply-token", expect.stringContaining("10分以内"));
  });

  it("連携済みなら再連携リンクを発行せず通知設定を案内する", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(prisma.lineConnection.findUnique).mockResolvedValue({
      id: 1,
      userId: "user-1",
      lineUserId: "U123",
      linkedAt: new Date(),
      createdAt: new Date(),
      updatedAt: new Date(),
    });
    vi.mocked(replyLineText).mockResolvedValue(undefined);

    const response = await POST(request([{
      type: "message",
      replyToken: "reply-token",
      source: { type: "user", userId: "U123" },
      message: { type: "text", text: "連携" },
    }]));

    expect(response.status).toBe(200);
    expect(issueLineLinkToken).not.toHaveBeenCalled();
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("すでに連携済み")
    );
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("https://juken-map.com/line/settings")
    );
  });

  it("有効なnonceでアプリユーザーとLINEユーザーを連携する", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(replyLineText).mockResolvedValue(undefined);
    vi.mocked(prisma.lineLinkNonce.findUnique).mockResolvedValue({
      nonce: "nonce-1",
      userId: "user-1",
      expiresAt: new Date(Date.now() + 60_000),
      createdAt: new Date(),
    });
    transactionPrisma.lineConnection.findUnique.mockResolvedValue(null);
    transactionPrisma.lineConnection.upsert.mockResolvedValue({});
    transactionPrisma.lineLinkNonce.delete.mockResolvedValue({});

    const response = await POST(request([{
      type: "accountLink",
      replyToken: "reply-token",
      source: { type: "user", userId: "U123" },
      link: { result: "ok", nonce: "nonce-1" },
    }]));

    expect(response.status).toBe(200);
    expect(transactionPrisma.lineConnection.upsert).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      create: { userId: "user-1", lineUserId: "U123" },
      update: { lineUserId: "U123", linkedAt: expect.any(Date) },
    });
    expect(transactionPrisma.lineLinkNonce.delete).toHaveBeenCalledWith({
      where: { nonce: "nonce-1" },
    });
    expect(replyLineText).toHaveBeenCalledWith("reply-token", expect.stringContaining("連携が完了"));
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("https://juken-map.com/line/settings")
    );
  });

  it("別ユーザーに連携済みなら解除方法を返信する", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(replyLineText).mockResolvedValue(undefined);
    vi.mocked(prisma.lineLinkNonce.findUnique).mockResolvedValue({
      nonce: "nonce-2",
      userId: "user-2",
      expiresAt: new Date(Date.now() + 60_000),
      createdAt: new Date(),
    });
    transactionPrisma.lineConnection.findUnique.mockResolvedValue({
      id: "connection-1",
      userId: "user-1",
      lineUserId: "U123",
      linkedAt: new Date(),
      createdAt: new Date(),
      updatedAt: new Date(),
    });
    transactionPrisma.lineLinkNonce.delete.mockResolvedValue({});

    const response = await POST(request([{
      type: "accountLink",
      replyToken: "reply-token",
      source: { type: "user", userId: "U123" },
      link: { result: "ok", nonce: "nonce-2" },
    }]));

    expect(response.status).toBe(200);
    expect(transactionPrisma.lineConnection.upsert).not.toHaveBeenCalled();
    expect(replyLineText).toHaveBeenCalledWith("reply-token", expect.stringContaining("別の受験マップアカウント"));
  });
});
