import type { FastifyInstance } from "fastify";
import { errorBody } from "./error-handling.ts";

/**
 * 同時に処理中の API リクエストの上限。
 *
 * 上限が無いと、捌ききれない分は DB の接続待ちに無制限に並び、並んだ全員が遅くなる。
 * 実測では限界を超えた途端に 5xx は出ないまま全 API の p95 が秒単位になり、
 * 処理できる件数そのものも落ちた（設定 250 RPS に対して実測 174 RPS）。
 * 上限を超えた分をすぐ断れば、受け付けた分は普段どおりの速さで返せる。
 *
 * 値は負荷試験（scripts/run-loadtest-limit.sh）で決めた。手元の実測（2026-09-23）で
 * 成功は毎秒約450件で頭打ちになる。64 だと上限なしで捌けていた 450 RPS でも4%を断り、
 * 160 なら 450 RPS はほぼ全件受け付けつつ、600 RPS でも成功分の p95 が 0.6 秒に収まった
 * （上限なしでは 4.5 秒）。本番の方が処理能力は低いので、OVERLOAD_MAX_IN_FLIGHT で下げられる。
 */
export const DEFAULT_MAX_IN_FLIGHT = 160;

/** 断った応答の Retry-After（秒）。すぐ再送されると混雑が続くので少し待たせる。 */
const RETRY_AFTER_SECONDS = 1;

/**
 * 同時処理数の上限を超えた API リクエストを 503 で断る。
 *
 * 429 ではなく 503 にしている。429 は「その送り手が送りすぎ」という意味で、
 * 一人ひとりは普通に使っていても全体として混んでいる今回の状況には当たらない。
 * 送り手ごとの制限（サインインの IP 単位制限）は別に持っている。
 *
 * 死活監視とデプロイのスモークテストが叩く /api/health は数えない。混んでいるだけで
 * 「落ちている」と判定されると、正常なイメージまでロールバックされる。
 */
export function registerOverloadProtection(
  app: FastifyInstance,
  maxInFlight = DEFAULT_MAX_IN_FLIGHT
) {
  let inFlight = 0;

  // Better Auth は後段の onRequest で hijack して自前で応答するため、onResponse が
  // 呼ばれない。どの経路でも必ず1回だけ戻せるよう、Node の応答の close で数を戻す
  // （close は返し終えたときにも、途中で切断されたときにも1回だけ来る）。
  app.addHook("onRequest", (request, reply, done) => {
    if (!request.url.startsWith("/api/") || request.url === "/api/health") return done();

    if (inFlight >= maxInFlight) {
      request.log.warn({ inFlight, maxInFlight }, "request shed: overloaded");
      reply
        .code(503)
        .header("Retry-After", String(RETRY_AFTER_SECONDS))
        .send(errorBody(503, "OVERLOADED", String(request.id)));
      return;
    }

    inFlight += 1;
    reply.raw.once("close", () => {
      inFlight -= 1;
    });
    done();
  });
}
