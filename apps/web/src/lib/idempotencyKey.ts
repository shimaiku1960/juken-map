import { ApiError } from "@/web/lib/api-client";

// 学習記録の作成の Idempotency-Key（学習 2-2、JUK-206）。
//
// 保存は成功したのに応答だけ失われると、画面には失敗に見える。そこで送り直したときに、サーバーが
// 「1回目と同じ操作」と分かるよう、同じキーを送る。キーを新しくするのは「保存された」と分かったときだけ。
// - 成功（201）: 保存された。次の記録は別の操作。
// - 422: このキーで保存済み（本文を変えて送り直した）。次の記録は別の操作。
// それ以外は同じキーのまま送り直す。4xx はサーバーが DB に書く前に断ったもの、5xx と通信の失敗
// （TypeError）は保存されたか分からないもの。
// 画面を閉じたり読み直したりするとキーは新しくなる（そこまでは守らない）。

export type IdempotencyKeyHolder = {
  // 今の操作のキー。無ければ作る。
  current: () => string;
  // 送った結果を受け取り、保存されたと分かればキーを捨てる。
  settle: (error?: unknown) => void;
};

export function createIdempotencyKeyHolder(
  newKey: () => string = () => crypto.randomUUID()
): IdempotencyKeyHolder {
  let key: string | null = null;
  return {
    current: () => (key ??= newKey()),
    settle: (error) => {
      if (error === undefined || isAlreadySaved(error)) key = null;
    },
  };
}

// isAlreadySaved は、同じキーで保存済みだったという応答か（422）。
export function isAlreadySaved(error: unknown): boolean {
  return error instanceof ApiError && error.status === 422;
}
