import { Resend } from "resend";

// 遅延初期化: モジュール読み込み時ではなく、実際に送信するときに生成する。
// build時(CIに RESEND_API_KEY が無い)に new Resend() が走って落ちるのを防ぐ。
let client: Resend | null = null;

export function getResend(): Resend {
  if (!client) {
    client = new Resend(process.env.RESEND_API_KEY);
  }
  return client;
}
