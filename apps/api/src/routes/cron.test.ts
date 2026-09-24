import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services/sendDailyNotifications", () => ({
  sendDailyNotifications: vi.fn(),
}));

const { sendDailyNotifications } = await import(
  "@/api/services/sendDailyNotifications"
);
const { registerCronRoutes } = await import("./cron.ts");
const { buildTestApp, request } = await import("../test-support.ts");

const app = buildTestApp(registerCronRoutes);

// 認証はセッションではなく共有シークレット（GitHub Actions から呼ばれる）。
const post = (secret: string, body: unknown) =>
  request(app, "POST", "/api/cron/daily-study-notifications", body, {
    authorization: `Bearer ${secret}`,
  });

beforeEach(() => {
  vi.clearAllMocks();
  process.env.DAILY_NOTIFICATION_SECRET = "test-secret";
});

describe("POST /api/cron/daily-study-notifications", () => {
  it("秘密値が一致しなければ401を返す", async () => {
    expect((await post("wrong", { slot: "morning" })).statusCode).toBe(401);
    expect(sendDailyNotifications).not.toHaveBeenCalled();
  });

  it("Authorization が無ければ401を返す", async () => {
    const res = await request(app, "POST", "/api/cron/daily-study-notifications", {
      slot: "morning",
    });
    expect(res.statusCode).toBe(401);
    expect(sendDailyNotifications).not.toHaveBeenCalled();
  });

  it("秘密値が未設定なら、何を送っても401を返す", async () => {
    delete process.env.DAILY_NOTIFICATION_SECRET;
    expect((await post("undefined", { slot: "morning" })).statusCode).toBe(401);
    expect((await post("", { slot: "morning" })).statusCode).toBe(401);
    expect(sendDailyNotifications).not.toHaveBeenCalled();
  });

  it("朝夜以外は400を返す", async () => {
    expect((await post("test-secret", { slot: "noon" })).statusCode).toBe(400);
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

    const res = await post("test-secret", { slot: "morning" });

    expect(res.statusCode).toBe(200);
    expect(sendDailyNotifications).toHaveBeenCalledWith("morning");
  });
});
