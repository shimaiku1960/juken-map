import { EMAIL_KINDS, type EmailKind, reserveEmailSend } from "@/api/infra/email-limits";
import { getResend } from "@/api/infra/resend";
import { withDeadline } from "@/api/infra/timeout";
import { logger } from "@/api/observability/logger";
import { countEmailSend, initEmailSendCounts, observeResendQuota } from "@/api/observability/metrics";
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

initEmailSendCounts(EMAIL_KINDS);

/**
 * 1通送る。宛先ごと・全体の上限（email-limits.ts）を超えるなら送らずに false を返す。
 * Resend は失敗しても例外にせず error を返すので、ここで例外に直す。
 */
async function send(kind: EmailKind, to: string, subject: string, html: string): Promise<boolean> {
  if (!(await reserveEmailSend(kind, to))) {
    countEmailSend(kind, "blocked");
    return false;
  }

  try {
    const { error, headers } = await withDeadline(
      getResend().emails.send({ from: FROM, to, subject, html }),
      `resend.${kind}`
    );
    observeResendQuota(headers);
    if (error) {
      throw new Error(error.message);
    }
  } catch (error) {
    countEmailSend(kind, "failed");
    throw error;
  }
  countEmailSend(kind, "sent");
  return true;
}

// 確認メールと再設定メールは、Better Auth が送り終えるのを待たずに呼ぶ（auth.ts の backgroundTasks）。
// 投げた例外は Better Auth がログに残す。
export async function sendVerificationEmail(to: string, url: string) {
  await send(
    "verification",
    to,
    "【受験マップ】メールアドレスの確認",
    `<p>以下のリンクをクリックしてメールアドレスを確認してください。</p>
<p><a href="${url}">メールアドレスを確認する</a></p>`
  );
}

export async function sendPasswordResetEmail(to: string, url: string) {
  await send(
    "password-reset",
    to,
    "【受験マップ】パスワードの再設定",
    `<p>以下のリンクからパスワードを再設定してください。</p>
<p><a href="${url}">パスワードを再設定する</a></p>`
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
    await send(
      "password-changed",
      to,
      "【受験マップ】パスワードが変更されました",
      `<p>受験マップのパスワードが変更されました。ほかの端末のログインはすべて解除しています。</p>
<p>心当たりがない場合は、すぐに以下からパスワードを再設定してください。</p>
<p><a href="${SITE_URL}/forgot-password">パスワードを再設定する</a></p>`
    );
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
    await send(
      "admin-new-user",
      to,
      "【受験マップ】新しいユーザーが登録しました",
      `<p>受験マップに新しいユーザーが登録しました。</p>
<dl>
  <dt>登録日時</dt><dd>${escapeHtml(registeredAt)}</dd>
  <dt>表示名</dt><dd>${escapeHtml(user.name)}</dd>
  <dt>メールアドレス</dt><dd>${escapeHtml(user.email)}</dd>
</dl>`
    );
  } catch (error) {
    logger.error(
      { err: error },
      "[registration-notification] Failed to send notification."
    );
  }
}
