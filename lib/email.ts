import { getResend } from "@/lib/resend";

const FROM = "受験マップ <noreply@juken-map.com>";

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