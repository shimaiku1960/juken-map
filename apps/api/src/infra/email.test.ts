import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { reserveEmailSend } from "@/api/infra/email-limits";
import { getResend } from "@/api/infra/resend";
import {
  notifyAdminOfNewUser,
  sendPasswordChangedNotice,
  sendPasswordResetEmail,
  sendVerificationEmail,
} from "@/api/infra/email";
import { registry } from "@/api/observability/metrics";
import { takeLogLines } from "@/api/test-support";

vi.mock("@/api/infra/resend", () => ({
  getResend: vi.fn(),
}));

// 上限の数え方は email-limits.test.ts が本物の DB で確かめる。ここでは「上限なら送らない」だけを見る。
vi.mock("@/api/infra/email-limits", () => ({
  EMAIL_KINDS: ["verification", "password-reset", "password-changed", "admin-new-user"],
  reserveEmailSend: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(reserveEmailSend).mockReset();
  vi.mocked(reserveEmailSend).mockResolvedValue(true);
});

const send = vi.fn();
const originalNotificationEmail = process.env.ADMIN_NOTIFICATION_EMAIL;

describe("送信量のメトリクス", () => {
  it("まだ送っていない組み合わせも 0 で出しておく（最初の1件を increase() が見落とさないように）", async () => {
    const text = await registry.metrics();
    expect(text).toContain('email_sends_total{kind="password-reset",result="blocked"} 0');
    expect(text).toContain('email_sends_total{kind="admin-new-user",result="sent"} 0');
  });
});

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

describe("sendPasswordChangedNotice", () => {
  beforeEach(() => {
    send.mockReset();
    vi.mocked(getResend).mockReturnValue({
      emails: { send },
    } as unknown as ReturnType<typeof getResend>);
  });

  it("本人のアドレスへ、再設定の画面へのリンクを付けて送る", async () => {
    send.mockResolvedValue({ data: { id: "email-id" }, error: null });

    await sendPasswordChangedNotice("user@example.com");

    expect(send).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({
        to: "user@example.com",
        subject: "【受験マップ】パスワードが変更されました",
        html: expect.stringContaining('href="https://juken-map.com/forgot-password"'),
      })
    );
  });

  it("送れなくても投げずにログへ残す（再設定ではこの後にセッションを消すため）", async () => {
    send.mockRejectedValue(new Error("Resend unavailable"));
    takeLogLines();

    await expect(sendPasswordChangedNotice("user@example.com")).resolves.toBeUndefined();
    expect(takeLogLines()).toEqual([
      expect.objectContaining({
        level: 50,
        msg: "[password-changed-notice] Failed to send notice.",
        err: expect.objectContaining({ message: "Resend unavailable" }),
      }),
    ]);
  });
});

describe("確認メール・再設定メール", () => {
  beforeEach(() => {
    send.mockReset();
    vi.mocked(getResend).mockReturnValue({
      emails: { send },
    } as unknown as ReturnType<typeof getResend>);
    registry.resetMetrics();
  });

  it("上限を超えるなら送らない（E1）", async () => {
    vi.mocked(reserveEmailSend).mockResolvedValue(false);

    await sendVerificationEmail("user@example.com", "https://juken-map.com/verify");
    await sendPasswordResetEmail("user@example.com", "https://juken-map.com/reset");

    expect(reserveEmailSend).toHaveBeenCalledWith("verification", "user@example.com");
    expect(reserveEmailSend).toHaveBeenCalledWith("password-reset", "user@example.com");
    expect(send).not.toHaveBeenCalled();
    // 止めたことは、送信量のアラート（H1）のために数える。
    const text = await registry.metrics();
    expect(text).toContain('email_sends_total{kind="verification",result="blocked"} 1');
    expect(text).toContain('email_sends_total{kind="password-reset",result="blocked"} 1');
  });

  it("Resend のエラーを例外にする（Better Auth がログに残す）", async () => {
    send.mockResolvedValue({ data: null, error: { message: "daily_quota_exceeded" }, headers: null });

    await expect(sendPasswordResetEmail("user@example.com", "https://juken-map.com/reset")).rejects.toThrow(
      "daily_quota_exceeded"
    );
    expect(await registry.metrics()).toContain('email_sends_total{kind="password-reset",result="failed"} 1');
  });

  it("応答ヘッダーの送信枠の使用数をメトリクスに写す", async () => {
    send.mockResolvedValue({
      data: { id: "email-id" },
      error: null,
      headers: { "x-resend-daily-quota": "42", "x-resend-monthly-quota": "1234" },
    });

    await sendVerificationEmail("user@example.com", "https://juken-map.com/verify");

    const text = await registry.metrics();
    expect(text).toContain('resend_quota_used{period="daily"} 42');
    expect(text).toContain('resend_quota_used{period="monthly"} 1234');
    expect(text).toMatch(/resend_quota_observed_timestamp_seconds\{period="daily"\} \d/);
    expect(text).toContain('email_sends_total{kind="verification",result="sent"} 1');
  });
});
