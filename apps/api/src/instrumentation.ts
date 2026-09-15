// OpenTelemetry のトレース。1回のリクエストの中で、どこに何 ms かかったか
// （Fastify のハンドラ・SQL 1本ずつ）を記録し、OTLP で送る（手元では Tempo）。
//
// ライブラリを差し替えて計測を仕込む仕組みなので、fastify・mysql2・pino が読み込まれる
// より先に動かす必要がある。そのため server.ts から import せず、起動時に
// `--import ./src/instrumentation.ts` で先に読ませる（apps/api の dev スクリプト）。
//
// OTEL_EXPORTER_OTLP_ENDPOINT を設定したときだけ有効にする。設定しなければ何もしない。
import { register } from "node:module";

if (process.env.OTEL_EXPORTER_OTLP_ENDPOINT) {
  // ESM の import を横取りできるようにするフック。対象はライブラリが名乗り出たもの
  // （fastify・mysql2 など）だけに絞り、それ以外のモジュールは素通りさせる。
  const { createAddHookMessageChannel } = await import("import-in-the-middle");
  const { registerOptions, waitForAllMessagesAcknowledged } =
    createAddHookMessageChannel();
  register("import-in-the-middle/hook.mjs", import.meta.url, registerOptions);

  const { NodeSDK } = await import("@opentelemetry/sdk-node");
  const { OTLPTraceExporter } = await import("@opentelemetry/exporter-trace-otlp-proto");
  const { HttpInstrumentation } = await import("@opentelemetry/instrumentation-http");
  const { MySQL2Instrumentation } = await import("@opentelemetry/instrumentation-mysql2");
  const { PinoInstrumentation } = await import("@opentelemetry/instrumentation-pino");
  const { FastifyOtelInstrumentation } = await import("@fastify/otel");
  const { RedactingSpanExporter, redactPath } = await import("./observability/redact.ts");

  const sdk = new NodeSDK({
    serviceName: "juken-map-api",
    // 送り先は OTEL_EXPORTER_OTLP_ENDPOINT（+ /v1/traces）。
    // HTTP と Fastify の計測は URL を ? 以降まで属性に入れるので、送る直前にトークンを取り除く。
    traceExporter: new RedactingSpanExporter(new OTLPTraceExporter()),
    instrumentations: [
      // リクエスト全体のスパン。Prometheus が5秒おきに読む /metrics は数えない。
      new HttpInstrumentation({
        ignoreIncomingRequestHook: (req) => req.url === "/metrics",
        // 名前は既定だと「GET」だけで、Fastify のルートが分かったときに「GET /api/x/:id」へ
        // 付け直される。Better Auth は onRequest で横取りしてルートを通らないので「GET」の
        // ままになる。先にトークンを伏せたパスで名付けておく。
        requestHook: (span, request) => {
          if ("url" in request && request.url) {
            span.updateName(`${request.method} ${redactPath(request.url)}`);
          }
        },
      }),
      // Fastify のフック・ハンドラごとのスパン。Fastify() を作るたびに自動で登録される。
      new FastifyOtelInstrumentation({ registerOnInitialization: true }),
      // SQL 1本ずつのスパン（Better Auth の SQL も mysql2 を通るので含まれる）。
      new MySQL2Instrumentation(),
      // ログの行に trace_id・span_id を足す。Grafana で Loki のログ ⇔ Tempo のトレースを行き来する鍵。
      // ログそのものは今まで通りファイル → Alloy → Loki で運ぶので、OpenTelemetry では送らない。
      new PinoInstrumentation({ disableLogSending: true }),
    ],
  });
  sdk.start();

  await waitForAllMessagesAcknowledged();
}
