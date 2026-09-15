import { createServer } from "node:http";
import type { FastifyInstance, FastifyRequest } from "fastify";
import { Counter, Histogram, Registry, collectDefaultMetrics } from "prom-client";
import { logger } from "@/api/observability/logger";

// Prometheus に読ませる数字（Metrics）。ログが「1件ずつの出来事」なのに対し、
// こちらは「数えた・測った結果」だけを持つ。Prometheus が数秒おきに /metrics を
// 読みに来て、その差分からリクエスト数やエラー率を計算する。

export const registry = new Registry();

// CPU（process_cpu_seconds_total）、メモリ（process_resident_memory_bytes）、
// イベントループの遅れ（nodejs_eventloop_lag_seconds）などは prom-client が集める。
collectDefaultMetrics({ register: registry });

const httpRequestsTotal = new Counter({
  name: "http_requests_total",
  help: "返したリクエストの数",
  labelNames: ["method", "route", "status_code"],
  registers: [registry],
});

const httpRequestDuration = new Histogram({
  name: "http_request_duration_seconds",
  help: "リクエストを受けてから返し終えるまでの時間",
  labelNames: ["method", "route", "status_code"],
  // 本番の実測は 0.2 秒前後（通信の準備込み）。p95 < 1 秒の目標を跨ぐよう細かめに切る。
  buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5],
  registers: [registry],
});

/**
 * メトリクスの route ラベルに入れる値。
 *
 * ラベルは値の組み合わせごとに別の時系列になるので、実際の URL（/api/study-logs/12）を
 * 入れると ID の数だけ増えて Prometheus が重くなる。ルートの型（/api/study-logs/:id）を使う。
 * 画面（SPA の HTML や assets）はファイル名にハッシュが入りデプロイごとに変わるので1つにまとめ、
 * Better Auth はルートを登録せず onRequest で横取りしているので、型が取れない分をまとめる。
 */
function routeLabel(request: FastifyRequest) {
  if (!request.url.startsWith("/api/")) return "(web)";
  if (request.url.startsWith("/api/auth/")) return "/api/auth/*";
  return request.routeOptions.url ?? "(unmatched)";
}

/** 全リクエストの件数と所要時間を数える。ルートより前に登録する。 */
export function registerMetrics(app: FastifyInstance) {
  app.addHook("onResponse", async (request, reply) => {
    const labels = {
      method: request.method,
      route: routeLabel(request),
      status_code: String(reply.statusCode),
    };
    httpRequestsTotal.inc(labels);
    httpRequestDuration.observe(labels, reply.elapsedTime / 1000);
  });
}

/**
 * Prometheus が読みに来る /metrics を、アプリとは別のポートで出す。
 *
 * アプリと同じポートに置くと、nginx 越しに誰でも内部の数字を読めてしまう。
 * 別ポートにしておけば、本番の docker run は 3000 番しか公開していないので外から届かない。
 * METRICS_PORT を設定したときだけ起動する（本番では今は起動しない）。
 */
export function startMetricsServer(port: number) {
  const server = createServer(async (req, res) => {
    if (req.url !== "/metrics") {
      res.statusCode = 404;
      res.end();
      return;
    }
    res.setHeader("Content-Type", registry.contentType);
    res.end(await registry.metrics());
  });
  server.listen(port, "0.0.0.0", () => {
    logger.info(`Metrics listening at http://0.0.0.0:${port}/metrics`);
  });
  return server;
}
