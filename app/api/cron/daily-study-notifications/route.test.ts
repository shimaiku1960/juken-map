import { beforeEach, describe, expect, it, vi } from "vitest";
import { POST } from "./route";
import { sendDailyNotifications } from "@/lib/sendDailyNotifications";

vi.mock("@/lib/sendDailyNotifications", () => ({
  sendDailyNotifications: vi.fn(),
}));

const request = (secret: string, body: unknown) =>
  ({
    headers: new Headers({ authorization: `Bearer ${secret}` }),
    json: async () => body,
  }) as unknown as Request;

beforeEach(() => {
  vi.clearAllMocks();
  process.env.DAILY_NOTIFICATION_SECRET = "test-secret";
});

describe("POST /api/cron/daily-study-notifications", () => {
  it("秘密値が一致しなければ401を返す", async () => {
    expect((await POST(request("wrong", { slot: "morning" }))).status).toBe(401);
  });

  it("朝夜以外は400を返す", async () => {
    expect((await POST(request("test-secret", { slot: "noon" }))).status).toBe(
      400
    );
  });

  it("指定スロットの配信処理を実行する", async () => {
    vi.mocked(sendDailyNotifications).mockResolvedValue({
      date: "2026-08-30",
      slot: "morning",
      eligible: 1,
      sent: 1,
      skipped: 0,
      failed: 0,
    });

    const response = await POST(request("test-secret", { slot: "morning" }));

    expect(response.status).toBe(200);
    expect(sendDailyNotifications).toHaveBeenCalledWith("morning");
  });
});
