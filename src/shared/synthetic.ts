// 合成ユーザー（実在しない利用者）のメールアドレスの決まり。
//
// 識別のためだけでなく安全装置でもある：シミュレーション用 API が触ってよい相手の判断と、
// 掃除の DELETE の網は、このアドレスの形で掛ける。
// 実際の利用者がこの形のアドレスで登録することは無い。

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
