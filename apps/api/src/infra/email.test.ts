import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getResend } from "@/api/infra/resend";
import { notifyAdminOfNewUser } from "@/api/infra/email";
import { takeLogLines } from "@/api/test-support";

vi.mock("@/api/infra/resend", () => ({
  getResend: vi.fn(),
}));

const send = vi.fn();
const originalNotificationEmail = process.env.ADMIN_NOTIFICATION_EMAIL;

describe("notifyAdminOfNewUser", () => {
  beforeEach(() => {
    process.env.ADMIN_NOTIFICATION_EMAIL = "owner@example.com";
    send.mockReset();
    vi.mocked(getResend).mockReturnValue({
      emails: { send },
    } as unknown as ReturnType<typeof getResend>);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    if (originalNotificationEmail === undefined) {
      delete process.env.ADMIN_NOTIFICATION_EMAIL;
    } else {
      process.env.ADMIN_NOTIFICATION_EMAIL = originalNotificationEmail;
    }
  });

  it("登録情報を運営者へ送信し、HTMLをエスケープする", async () => {
    send.mockResolvedValue({ data: { id: "email-id" }, error: null });

    await notifyAdminOfNewUser({
      name: '<script>alert("x")</script>',
      email: "new-user@example.com",
      createdAt: new Date("2026-08-10T06:30:00.000Z"),
    });

    expect(send).toHaveBeenCalledOnce();
    expect(send).toHaveBeenCalledWith(
      expect.objectContaining({
        to: "owner@example.com",
        subject: "【受験マップ】新しいユーザーが登録しました",
        html: expect.stringContaining(
          "&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;"
        ),
      })
    );
  });

  it("通知先が未設定なら送信せず警告する", async () => {
    delete process.env.ADMIN_NOTIFICATION_EMAIL;
    takeLogLines();

    await notifyAdminOfNewUser({
      name: "新規ユーザー",
      email: "new-user@example.com",
      createdAt: new Date(),
    });

    expect(send).not.toHaveBeenCalled();
    expect(takeLogLines()).toEqual([
      expect.objectContaining({
        level: 40,
        msg: "[registration-notification] ADMIN_NOTIFICATION_EMAIL is not configured.",
      }),
    ]);
  });

  it("Resendのエラーで登録処理を失敗させない", async () => {
    send.mockResolvedValue({ data: null, error: { message: "Resend unavailable" } });
    takeLogLines();

    await expect(
      notifyAdminOfNewUser({
        name: "新規ユーザー",
        email: "new-user@example.com",
        createdAt: new Date(),
      })
    ).resolves.toBeUndefined();
    expect(takeLogLines()).toEqual([
      expect.objectContaining({
        level: 50,
        msg: "[registration-notification] Failed to send notification.",
        err: expect.objectContaining({ message: "Resend unavailable" }),
      }),
    ]);
  });
});

describe("合成ユーザー（シミュレーション）", () => {
  beforeEach(() => {
    process.env.ADMIN_NOTIFICATION_EMAIL = "owner@example.com";
    send.mockReset();
    vi.mocked(getResend).mockReturnValue({
      emails: { send },
    } as unknown as ReturnType<typeof getResend>);
  });

  it("登録しても運営者へは通知しない", async () => {
    await notifyAdminOfNewUser({
      name: "合成",
      email: "delivered+sim00001@resend.dev",
      createdAt: new Date(),
    });

    expect(send).not.toHaveBeenCalled();
  });
});
