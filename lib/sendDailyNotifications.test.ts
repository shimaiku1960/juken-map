import { beforeEach, describe, expect, it, vi } from "vitest";
import prisma from "@/lib/prisma";
import { getResend } from "@/lib/resend";
import { sendDailyNotifications } from "@/lib/sendDailyNotifications";
import { pushLineText } from "@/lib/line";

vi.mock("@/lib/prisma", () => ({
  default: {
    user: { findMany: vi.fn() },
    notificationDelivery: { create: vi.fn(), delete: vi.fn() },
  },
}));
vi.mock("@/lib/resend", () => ({ getResend: vi.fn() }));
vi.mock("@/lib/line", () => ({ pushLineText: vi.fn() }));

const findUsers = vi.mocked(prisma.user.findMany);
const createDelivery = vi.mocked(prisma.notificationDelivery.create);
const deleteDelivery = vi.mocked(prisma.notificationDelivery.delete);
const send = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getResend).mockReturnValue({
    emails: { send },
  } as unknown as ReturnType<typeof getResend>);
});

describe("sendDailyNotifications", () => {
  it("有効なユーザーへ送り、配信履歴を先に確保する", async () => {
    findUsers.mockResolvedValue([
      {
        id: "user-1",
        email: "me@example.com",
        name: "User",
        nickname: "育朗",
        notificationPreference: { morningEnabled: false, eveningEnabled: true, lineMorningEnabled: false, lineEveningEnabled: false },
        lineConnection: null,
        studyPlans: [],
        studyLogs: [{ minutes: 30 }],
      },
    ] as never);
    createDelivery.mockResolvedValue({ id: 10 } as never);
    send.mockResolvedValue({ data: { id: "mail-1" }, error: null });

    const result = await sendDailyNotifications(
      "evening",
      new Date("2026-08-30T12:00:00.000Z")
    );

    expect(result).toMatchObject({ sent: 1, skipped: 0, failed: 0 });
    expect(createDelivery).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ userId: "user-1", slot: "evening", channel: "email" }),
      })
    );
    expect(send).toHaveBeenCalledWith(
      expect.objectContaining({ to: "me@example.com" })
    );
  });

  it("同日の重複は送信をスキップする", async () => {
    findUsers.mockResolvedValue([
      {
        id: "user-1",
        email: "me@example.com",
        name: "User",
        nickname: null,
        notificationPreference: { morningEnabled: true, eveningEnabled: false, lineMorningEnabled: false, lineEveningEnabled: false },
        lineConnection: null,
        studyPlans: [],
        studyLogs: [],
      },
    ] as never);
    createDelivery.mockRejectedValue({ code: "P2002" });

    const result = await sendDailyNotifications("morning");

    expect(result.skipped).toBe(1);
    expect(send).not.toHaveBeenCalled();
  });

  it("送信失敗時は履歴を削除して再試行可能にする", async () => {
    findUsers.mockResolvedValue([
      {
        id: "user-1",
        email: "me@example.com",
        name: "User",
        nickname: null,
        notificationPreference: { morningEnabled: true, eveningEnabled: false, lineMorningEnabled: false, lineEveningEnabled: false },
        lineConnection: null,
        studyPlans: [],
        studyLogs: [],
      },
    ] as never);
    createDelivery.mockResolvedValue({ id: 10 } as never);
    send.mockResolvedValue({ data: null, error: { message: "unavailable" } });
    deleteDelivery.mockResolvedValue({ id: 10 } as never);

    const result = await sendDailyNotifications("morning");

    expect(result.failed).toBe(1);
    expect(deleteDelivery).toHaveBeenCalledWith({ where: { id: 10 } });
  });

  it("メールとLINEをそれぞれ独立して送信する", async () => {
    findUsers.mockResolvedValue([{
      id: "user-1",
      email: "me@example.com",
      name: "User",
      nickname: null,
      notificationPreference: { morningEnabled: true, eveningEnabled: false, lineMorningEnabled: true, lineEveningEnabled: false },
      lineConnection: { lineUserId: "U123" },
      studyPlans: [],
      studyLogs: [],
    }] as never);
    createDelivery.mockResolvedValueOnce({ id: 10 } as never).mockResolvedValueOnce({ id: 11 } as never);
    send.mockResolvedValue({ data: { id: "mail-1" }, error: null });
    vi.mocked(pushLineText).mockResolvedValue(undefined);

    const result = await sendDailyNotifications("morning");

    expect(result).toMatchObject({ eligible: 2, sent: 2, failed: 0 });
    expect(createDelivery).toHaveBeenNthCalledWith(1, expect.objectContaining({ data: expect.objectContaining({ channel: "email" }) }));
    expect(createDelivery).toHaveBeenNthCalledWith(2, expect.objectContaining({ data: expect.objectContaining({ channel: "line" }) }));
    expect(pushLineText).toHaveBeenCalledWith("U123", expect.stringContaining("おはようございます"));
  });
});
