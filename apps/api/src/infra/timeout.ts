// 外部サービス（LINE・microCMS・Resend）の呼び出しに、待ち時間の上限を設ける。
//
// 上限が無いと、相手が落ちずに黙り込んだとき——応答を返さないまま接続を保つとき——
// こちらのリクエストも返らない。Node は1プロセスで、DB のプールも connectionLimit: 5 しか
// 無いので、そういう待ちが数本たまるだけでアプリ全体が詰まる。
// 相手の障害をこちらの障害にしないための線引きがここ。

/**
 * 既定の上限。相手の普段の遅さ（数百ms）は吸収しつつ、詰まったときは早く諦める。
 */
export const EXTERNAL_TIMEOUT_MS = 5_000;

/**
 * fetch に渡す signal を作る。
 *
 * 呼び出し側が自分の signal を持っているときは、どちらか早く止まった方で中断する
 * （例：LINE 連携完了メッセージは routes/line.ts が 3 秒で切っている）。
 */
export function abortAfter(
  ms: number = EXTERNAL_TIMEOUT_MS,
  caller?: AbortSignal | null
): AbortSignal {
  const timeout = AbortSignal.timeout(ms);
  return caller ? AbortSignal.any([caller, timeout]) : timeout;
}

/**
 * signal を受け取れない SDK 用に、待つのをやめる時間を決める。
 *
 * Resend の SDK は fetch の設定を外から渡せない（ResendOptions は baseUrl と userAgent
 * だけ）。通信そのものは止められないので、止められる相手には abortAfter を使うこと。
 * ここで止まるのは「待つ側」だけで、送信自体は裏で続く可能性がある。
 */
export function withDeadline<T>(
  promise: Promise<T>,
  label: string,
  ms: number = EXTERNAL_TIMEOUT_MS
): Promise<T> {
  let timer: ReturnType<typeof setTimeout>;
  const deadline = new Promise<never>((_, reject) => {
    timer = setTimeout(
      () => reject(new Error(`${label} timed out after ${ms}ms`)),
      ms
    );
  });
  // タイマーを消さないと、送信が先に終わってもプロセスが上限時間まで終われない。
  return Promise.race([promise, deadline]).finally(() => clearTimeout(timer));
}
