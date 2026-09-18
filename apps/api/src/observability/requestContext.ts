import { AsyncLocalStorage } from "node:async_hooks";

// リクエストごとの文脈を、引数で引き回さずに深い場所から読めるようにする。
//
// measured() は services の45か所から呼ばれ、db.ts の run() はさらにその下にある。
// reqId を届けるためにこの全部へ引数を足すと、業務ロジックの関数シグネチャに
// ログの都合が漏れる（createStudyLog(userId, input, logger) のような形になる）。
//
// AsyncLocalStorage は、非同期の呼び出しをまたいでも「今どのリクエストの処理中か」を
// 保ってくれる Node 標準の仕組みで、まさにこの用途のためにある。入れるのは
// server.ts の onRequest フック1か所だけで、呼び出し側は何も変えなくてよい。

type RequestContext = {
  /** Fastify が採番した reqId（genReqId）。ログを1本の線に束ねるための鍵。 */
  reqId: string;
  /**
   * シミュレーション（sim/）からのリクエストなら true。X-Sim-Run ヘッダーで判断する。
   * ログに sim:true が付くので、Grafana で実ユーザーと合成を分けて読める。
   */
  sim?: boolean;
};

const storage = new AsyncLocalStorage<RequestContext>();

/**
 * fn の実行中（そこから派生する非同期処理も含む）を、この文脈の中に置く。
 *
 * Fastify のフックは done() を呼ぶと残りの処理へ進むので、done をこの中で
 * 呼べば、そのリクエストの処理全体がこの文脈に入る。
 */
export function runWithRequestContext<T>(
  context: RequestContext,
  fn: () => T
): T {
  return storage.run(context, fn);
}

/**
 * 今処理中のリクエストの reqId。リクエストの外（起動時、cron、seed など）では
 * undefined を返す。pino は値が undefined のキーを書き出さないので、そのまま渡してよい。
 */
export function currentReqId(): string | undefined {
  return storage.getStore()?.reqId;
}

/** 今処理中のリクエストがシミュレーションからなら true。それ以外は undefined（ログに出さない）。 */
export function currentSim(): true | undefined {
  return storage.getStore()?.sim ? true : undefined;
}
