import type { TransportItem } from "@grafana/faro-web-sdk";

// 画面（ブラウザ）で起きたエラーと Web Vitals を Grafana Cloud（Frontend Observability）へ送る。
// ブラウザのエラーは Fastify に届かないので、サーバー側のログ・トレースでは見えない。
//
// 送り先はサーバーが本番の環境変数 FARO_COLLECTOR_URL から <meta> で差し込む（apps/api-go/seo.go）。
// GA4 と同じく、バンドルに焼き込まず実行時の設定のまま扱うため。meta が無い環境（手元の開発・
// テスト）では何もしない。
const COLLECTOR_META = "faro-collector-url";

// URL に載る秘密の値。サーバー側のログ・トレースと同じく、外へ出す前に伏せる。
// - /reset-password?token=…（パスワード再設定）、/verify-email/confirm?token=…（メール確認）
//   画面は読み込んだらすぐ URL からトークンを消すが（useTokenFromLink.ts）、その前に起きたエラーのため
// - /line/link?linkToken=…（LINE 連携）
// - /api/auth/reset-password/<token>（以前の Better Auth のリンク。トークンがパスにある）
// - 上の URL がエンコードされて別の URL のクエリに入ったもの（linkToken%3D…）。Faro は読み込んだ
//   通信の URL も送るので、GA4 への送信（dl=ページの URL）を通して入ってくる（JUK-124）
const SECRET_PATTERNS: [RegExp, string][] = [
  [/([?&#](?:token|linkToken)=)[^&#\s"'<>]+/gi, "$1[REDACTED]"],
  [/((?:%3F|%26|%23)(?:token|linkToken)%3D)(?:(?!%26|%23)[^&#\s"'<>])+/gi, "$1[REDACTED]"],
  [/(\/api\/auth\/reset-password\/)[^/?#\s"'<>]+/g, "$1[REDACTED]"],
];

export function redactSecrets<T>(value: T): T {
  if (typeof value === "string") {
    return SECRET_PATTERNS.reduce(
      (text, [pattern, replacement]) => text.replace(pattern, replacement),
      value as string
    ) as T;
  }
  if (Array.isArray(value)) {
    return value.map((item) => redactSecrets(item)) as T;
  }
  if (value !== null && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, redactSecrets(item)])
    ) as T;
  }
  return value;
}

export function readCollectorUrl(doc: Document = document): string | null {
  const content = doc
    .querySelector(`meta[name="${COLLECTOR_META}"]`)
    ?.getAttribute("content");
  return content ? content : null;
}

// 最初の表示を遅らせないよう、SDK は描画のあとで読み込む（別チャンクになる）。
// そのため読み込みが終わる前に起きたエラーは拾えないが、最初の JS を増やさないほうを優先する。
export async function startFaro() {
  const url = readCollectorUrl();
  if (!url) return;

  const { initializeFaro, getWebInstrumentations } = await import(
    "@grafana/faro-web-sdk"
  );
  initializeFaro({
    url,
    app: { name: "juken-map-web", environment: "production" },
    instrumentations: getWebInstrumentations(),
    beforeSend: (item: TransportItem) => redactSecrets(item),
  });
}
