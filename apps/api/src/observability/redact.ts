import type { Attributes } from "@opentelemetry/api";
import type { tracing } from "@opentelemetry/sdk-node";

// ログとトレースに残す URL から、トークンを取り除く。
//
// このファイルは pino・fastify などの計測対象を import しない。instrumentation.ts が
// 計測を仕込む前に読み込むので、ここで pino を読むと pino の計測（trace_id の付与）が効かなくなる。

/**
 * URL のパスからトークンを取り除く。
 *
 * メール確認・パスワード再設定のリンクや OAuth の戻り先は、URL の ? 以降に
 * トークンや code が乗る。そのまま残すと、ログやトレースを読める人がそのリンクを
 * 使えてしまう。パスワード再設定だけはトークンがパスに入る
 * （Better Auth の /api/auth/reset-password/:token）ので、そこも伏せる。
 */
export function redactPath(url: string) {
  const [pathname] = url.split("?");
  return pathname.replace(/^(\/api\/auth\/reset-password\/)[^/]+/, "$1:token");
}

// パスとクエリが入る属性（新旧の命名規則の両方）。HTTP の計測と Fastify の計測が付ける。
const PATH_KEYS = ["url.path", "http.target"];
const FULL_URL_KEYS = ["url.full", "http.url"];
const QUERY_KEYS = ["url.query"];

/** スパンの属性のうち、URL に当たるものからトークンを取り除いた写しを返す。 */
export function redactUrlAttributes(attributes: Attributes): Attributes {
  const result: Attributes = { ...attributes };
  for (const key of PATH_KEYS) {
    const value = result[key];
    if (typeof value === "string") result[key] = redactPath(value);
  }
  for (const key of FULL_URL_KEYS) {
    const value = result[key];
    if (typeof value === "string") {
      const url = new URL(value);
      result[key] = url.origin + redactPath(url.pathname);
    }
  }
  for (const key of QUERY_KEYS) delete result[key];
  return result;
}

/**
 * 送る直前に、全スパンの URL からトークンを取り除く送信役。
 *
 * 計測ライブラリごとのフックで直すと、あとで計測を足したときに漏れる。送り出す1か所で
 * まとめて直す。スパン本体は書き換えず、属性だけ差し替えた写しを渡す
 * （Object.create で元のスパンを親にするので、spanContext() などはそのまま使える）。
 */
export class RedactingSpanExporter implements tracing.SpanExporter {
  constructor(private readonly inner: tracing.SpanExporter) {}

  export(
    spans: tracing.ReadableSpan[],
    resultCallback: Parameters<tracing.SpanExporter["export"]>[1]
  ) {
    const redacted = spans.map(
      (span) =>
        Object.create(span, {
          attributes: { value: redactUrlAttributes(span.attributes) },
        }) as tracing.ReadableSpan
    );
    this.inner.export(redacted, resultCallback);
  }

  shutdown() {
    return this.inner.shutdown();
  }

  forceFlush() {
    return this.inner.forceFlush?.() ?? Promise.resolve();
  }
}
