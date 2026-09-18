import { getResend } from "@/api/infra/resend";
import { withDeadline } from "@/api/infra/timeout";
import { logger } from "@/api/observability/logger";
import { isSyntheticEmail } from "@/shared/synthetic";

const FROM = "受験マップ <noreply@juken-map.com>";

type RegisteredUser = {
  name: string;
  email: string;
  createdAt: Date;
};

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

export async function sendVerificationEmail(to: string, url: string) {
  // 登録・ログインの流れの中で待たされるので、Resend が詰まったら諦める。
  await withDeadline(
    getResend().emails.send({
      from: FROM,
      to,
      subject: "【受験マップ】メールアドレスの確認",
      html: `<p>以下のリンクをクリックしてメールアドレスを確認してください。</p>
<p><a href="${url}">メールアドレスを確認する</a></p>`,
    }),
    "resend.sendVerificationEmail"
  );
}

export async function sendPasswordResetEmail(to: string, url: string) {
  await withDeadline(
    getResend().emails.send({
      from: FROM,
      to,
      subject: "【受験マップ】パスワードの再設定",
      html: `<p>以下のリンクからパスワードを再設定してください。</p>
<p><a href="${url}">パスワードを再設定する</a></p>`,
    }),
    "resend.sendPasswordResetEmail"
  );
}

/**
 * 新規登録は通知の成否にかかわらず完了させる。
 * 通知先の設定漏れやResendの障害はサーバーログで検知する。
 */
export async function notifyAdminOfNewUser(user: RegisteredUser) {
  // シミュレーションの合成ユーザーの登録は運営者に知らせない。毎日何十通も届くうえ、
  // Resend の送信枠（無料枠は1日100通）を実ユーザーの確認メールと取り合ってしまう。
  // 合成ユーザー本人への確認メールは、実際の動線と同じく普通に送る。
  if (isSyntheticEmail(user.email)) return;

  const to = process.env.ADMIN_NOTIFICATION_EMAIL;

  if (!to) {
    logger.warn(
      "[registration-notification] ADMIN_NOTIFICATION_EMAIL is not configured."
    );
    return;
  }

  const registeredAt = new Intl.DateTimeFormat("ja-JP", {
    dateStyle: "long",
    timeStyle: "short",
    timeZone: "Asia/Tokyo",
  }).format(user.createdAt);

  try {
    const { error } = await withDeadline(
      getResend().emails.send({
        from: FROM,
        to,
        subject: "【受験マップ】新しいユーザーが登録しました",
        html: `<p>受験マップに新しいユーザーが登録しました。</p>
<dl>
  <dt>登録日時</dt><dd>${escapeHtml(registeredAt)}</dd>
  <dt>表示名</dt><dd>${escapeHtml(user.name)}</dd>
  <dt>メールアドレス</dt><dd>${escapeHtml(user.email)}</dd>
</dl>`,
      }),
      "resend.notifyAdminOfNewUser"
    );

    if (error) {
      throw new Error(error.message);
    }
  } catch (error) {
    logger.error(
      { err: error },
      "[registration-notification] Failed to send notification."
    );
  }
}
