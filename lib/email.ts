import { getResend } from "@/lib/resend";

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
  await getResend().emails.send({
    from: FROM,
    to,
    subject: "【受験マップ】メールアドレスの確認",
    html: `<p>以下のリンクをクリックしてメールアドレスを確認してください。</p>
<p><a href="${url}">メールアドレスを確認する</a></p>`,
  });
}

export async function sendPasswordResetEmail(to: string, url: string) {
  await getResend().emails.send({
    from: FROM,
    to,
    subject: "【受験マップ】パスワードの再設定",
    html: `<p>以下のリンクからパスワードを再設定してください。</p>
<p><a href="${url}">パスワードを再設定する</a></p>`,
  });
}

/**
 * 新規登録は通知の成否にかかわらず完了させる。
 * 通知先の設定漏れやResendの障害はサーバーログで検知する。
 */
export async function notifyAdminOfNewUser(user: RegisteredUser) {
  const to = process.env.ADMIN_NOTIFICATION_EMAIL;

  if (!to) {
    console.warn(
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
    const { error } = await getResend().emails.send({
      from: FROM,
      to,
      subject: "【受験マップ】新しいユーザーが登録しました",
      html: `<p>受験マップに新しいユーザーが登録しました。</p>
<dl>
  <dt>登録日時</dt><dd>${escapeHtml(registeredAt)}</dd>
  <dt>表示名</dt><dd>${escapeHtml(user.name)}</dd>
  <dt>メールアドレス</dt><dd>${escapeHtml(user.email)}</dd>
</dl>`,
    });

    if (error) {
      throw new Error(error.message);
    }
  } catch (error) {
    console.error("[registration-notification] Failed to send notification.", error);
  }
}
