import { getResend } from "@/api/infra/resend";
import { withDeadline } from "@/api/infra/timeout";
import { logger } from "@/api/observability/logger";
import { SITE_URL } from "@/shared/site";

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
 * パスワードが変わったことを本人に知らせる（変更・再設定のどちらも。セキュリティ基準 B6）。
 * 乗っ取った人がパスワードを変えたとき、本人が気づいて再設定できるようにする。
 *
 * 変更はもう済んでいるので、送れなくても失敗にせずログに残すだけにする。
 * 再設定では Better Auth がこの後にセッションを消すので、ここで投げるとその削除が飛ばされる。
 */
export async function sendPasswordChangedNotice(to: string) {
  try {
    const { error } = await withDeadline(
      getResend().emails.send({
        from: FROM,
        to,
        subject: "【受験マップ】パスワードが変更されました",
        html: `<p>受験マップのパスワードが変更されました。ほかの端末のログインはすべて解除しています。</p>
<p>心当たりがない場合は、すぐに以下からパスワードを再設定してください。</p>
<p><a href="${SITE_URL}/forgot-password">パスワードを再設定する</a></p>`,
      }),
      "resend.sendPasswordChangedNotice"
    );
    if (error) {
      throw new Error(error.message);
    }
  } catch (error) {
    logger.error({ err: error }, "[password-changed-notice] Failed to send notice.");
  }
}

/**
 * 新規登録は通知の成否にかかわらず完了させる。
 * 通知先の設定漏れやResendの障害はサーバーログで検知する。
 */
export async function notifyAdminOfNewUser(user: RegisteredUser) {
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
