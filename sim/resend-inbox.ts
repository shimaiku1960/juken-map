// Resend に送られた確認メールを読み、中のリンクを取り出す。
//
// シミュレーションの利用者は Resend のテスト用アドレス（delivered+simNNNNN@resend.dev）で登録する。
// アプリは実際の利用者と同じコードで確認メールを送り、Resend は本当に送信処理をして
// 「届いた」扱いにする。受信箱は無いので、送ったメールの本文を Resend の API で読む。
//
// 読むにはフルアクセスのキーが要る（送信専用のキーでは一覧も本文も 401）。
// アプリが送信に使うキーとは分け、シミュレータだけに持たせる（RESEND_READ_API_KEY）。
//
// Resend の API は1チームで毎秒10回まで。アプリの送信と枠を分け合うので、ゆっくり読む。

type ListedEmail = { id: string; to: string[]; subject: string; created_at: string };

const API = "https://api.resend.com";
const POLL_INTERVAL_MS = 2000;
const TIMEOUT_MS = 60_000;

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

export class ResendInbox {
  constructor(private readonly apiKey: string) {}

  private async get<T>(path: string): Promise<T> {
    const response = await fetch(`${API}${path}`, {
      headers: { Authorization: `Bearer ${this.apiKey}` },
    });
    if (!response.ok) {
      throw new Error(`Resend ${path} → ${response.status} ${(await response.text()).slice(0, 200)}`);
    }
    return (await response.json()) as T;
  }

  /**
   * email 宛てに since 以降に送られた確認メールを探し、確認リンクのパス（/api/auth/verify-email?...）を返す。
   * リンクのホストはサーバーの BETTER_AUTH_URL なので、パスだけ返して接続先はシミュレータが決める。
   */
  async verificationPath(email: string, since: Date): Promise<string> {
    const deadline = Date.now() + TIMEOUT_MS;
    while (Date.now() < deadline) {
      const { data } = await this.get<{ data: ListedEmail[] }>("/emails?limit=50");
      const found = data.find(
        (mail) =>
          mail.to.some((to) => to.toLowerCase() === email.toLowerCase()) &&
          mail.subject.includes("メールアドレスの確認") &&
          new Date(mail.created_at).getTime() >= since.getTime() - 5000
      );
      if (found) {
        const { html } = await this.get<{ html: string | null }>(`/emails/${found.id}`);
        return extractVerificationPath(html ?? "");
      }
      await sleep(POLL_INTERVAL_MS);
    }
    throw new Error(`${email} 宛ての確認メールが ${TIMEOUT_MS / 1000} 秒以内に見つかりませんでした`);
  }
}

/** メール本文の HTML から確認リンクを取り出し、パスとクエリだけにする。 */
export function extractVerificationPath(html: string): string {
  const match = html.match(/href="([^"]*\/api\/auth\/verify-email\?[^"]*)"/);
  if (!match) throw new Error("確認メールの本文に確認リンクがありません");
  const href = match[1].replaceAll("&amp;", "&");
  const url = new URL(href);
  return url.pathname + url.search;
}
