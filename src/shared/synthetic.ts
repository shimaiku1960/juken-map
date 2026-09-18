// 合成ユーザー（実在しない利用者）のメールアドレスの決まり。
//
// 識別のためだけでなく安全装置でもある：運営者へ通知しない判断、シミュレーション用 API が
// 触ってよい相手の判断、掃除の DELETE の網は、すべてこのアドレスの形で掛ける。
// 実際の利用者がこの形のアドレスで登録することは無い。

// 手元の seed（db/seed-synthetic.ts）が作る利用者。
// .invalid は「実在しないことが保証されたTLD」（RFC 2606）で、メールはどこにも届かない。
export const SEED_EMAIL_DOMAIN = "@synthetic.juken-map.invalid";

// シミュレーション（sim/）が作る利用者。実際の利用者と同じく確認メールを受け取る必要があるので、
// Resend のテスト用アドレスを使う。Resend は本当に送信処理をして「届いた」扱いにし、
// 送ったメールの本文は Resend の API で読める。+ 以降はラベルで、1人ずつ別のアドレスになる。
export function simEmailFor(seq: number) {
  return `delivered+sim${String(seq).padStart(5, "0")}@resend.dev`;
}

/** SQL の LIKE で「シミュレーションの利用者」だけを選ぶ条件。 */
export const SIM_EMAIL_LIKE = "delivered+sim%@resend.dev";

const SIM_EMAIL_PATTERN = /^delivered\+sim\d+@resend\.dev$/;

export function isSimEmail(email: string | null | undefined): boolean {
  return typeof email === "string" && SIM_EMAIL_PATTERN.test(email.toLowerCase());
}

/** seed かシミュレーションの合成ユーザーなら true。 */
export function isSyntheticEmail(email: string | null | undefined): boolean {
  if (typeof email !== "string") return false;
  return email.toLowerCase().endsWith(SEED_EMAIL_DOMAIN) || isSimEmail(email);
}
