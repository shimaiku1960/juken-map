import type { FastifyInstance } from "fastify";
import { inlineScriptHashes } from "./seo.ts";

// ブラウザに「このサイトをどう扱うか」を伝えるヘッダー。ブラウザは多くの防御を持っているが、
// サイトが宣言しないと働かないものが多い。
//
// nginx ではなくアプリで付けるのは、本番の nginx の設定がサーバー上に手で置かれていて
// リポジトリで管理できず、テストもできないため。

/** CSP の違反の報告を受ける先（routes/csp-report.ts）。 */
export const CSP_REPORT_PATH = "/api/csp-report";

/**
 * 画面が読み込んでよいものの一覧（Content-Security-Policy）。
 *
 * 今は Report-Only で送る＝止めずに違反を報告させるだけ。許可の漏れがあると画面が壊れるため、
 * 本番の報告（Grafana の Loki で `csp violation`）を見て一覧を直してから、止めるモードに切り替える。
 */
export function contentSecurityPolicy(env: NodeJS.ProcessEnv = process.env) {
  // Faro（画面のエラーの送り先）。apps/web/src/lib/faro.ts が使う。
  const faroOrigin = env.FARO_COLLECTOR_URL ? new URL(env.FARO_COLLECTOR_URL).origin : null;
  // GA4 の計測の送り先。gtag.js は地域ごとのサブドメインへ送る。
  const googleAnalytics = [
    "https://www.googletagmanager.com",
    "https://*.google-analytics.com",
    "https://*.analytics.google.com",
  ];

  const directives: Record<string, string[]> = {
    "default-src": ["'self'"],
    "script-src": ["'self'", "https://www.googletagmanager.com", ...inlineScriptHashes()],
    // Sonner（トースト）が JS で <style> を差し込むため、スタイルだけはインラインを許す。
    // CSS の差し込みでできることはスクリプトよりずっと限られるので、よくある妥協。
    // スクリプトは 'unsafe-inline' にせず、GA4 の1本だけをハッシュで許す。
    "style-src": ["'self'", "'unsafe-inline'", "https://fonts.googleapis.com"],
    "font-src": ["'self'", "https://fonts.gstatic.com"],
    // microCMS の記事画像、GA4 の計測ピクセル。
    "img-src": ["'self'", "data:", "blob:", "https://images.microcms-assets.io", ...googleAnalytics],
    "connect-src": ["'self'", ...(faroOrigin ? [faroOrigin] : []), ...googleAnalytics],
    "object-src": ["'none'"],
    "base-uri": ["'self'"],
    "form-action": ["'self'"],
    // 他のサイトの <iframe> に入れさせない（クリックジャッキング対策。X-Frame-Options の後継）。
    "frame-ancestors": ["'none'"],
    // 報告は report-uri だけにする。後継の report-to（Reporting API）を並べると Chrome はそちらを
    // 使うが、報告をまとめて後から送るため、手元の確認では数分待っても1件も届かなかった。
    // report-uri なら Chrome・Safari・Firefox のどれも違反のたびにすぐ送る。
    "report-uri": [CSP_REPORT_PATH],
  };

  return Object.entries(directives)
    .map(([name, values]) => `${name} ${values.join(" ")}`)
    .join("; ");
}

export function securityHeaders(env: NodeJS.ProcessEnv = process.env): Record<string, string> {
  return {
    // 以後1年は必ず https でつなぐ（http に落とされて盗み見られるのを防ぐ）。
    // サブドメインは Resend の送信用（send.）など Web でないものがあるので含めない。
    "Strict-Transport-Security": "max-age=31536000",
    // 中身の種類をブラウザに推測させない（画像のふりをしたファイルを JS として実行させない）。
    "X-Content-Type-Options": "nosniff",
    // 他のサイトの枠に入れさせない。frame-ancestors を解さない古いブラウザ向けに残す。
    "X-Frame-Options": "DENY",
    // 他のサイトへ移るときは、オリジンだけ伝える（URL のトークンを外へ出さない）。
    "Referrer-Policy": "strict-origin-when-cross-origin",
    "Content-Security-Policy-Report-Only": contentSecurityPolicy(env),
  };
}

/**
 * すべての応答にセキュリティヘッダーを付ける。
 *
 * onSend ではなく onRequest で生の応答（reply.raw）に積むのは、/api/auth/* が Better Auth へ
 * hijack されて Fastify の応答処理を通らないため。Node は setHeader した値と、あとで
 * writeHead に渡される値をまとめて送るので、静的ファイル・SPA・API・Better Auth のどれにも付く。
 */
export function registerSecurityHeaders(app: FastifyInstance) {
  const headers = Object.entries(securityHeaders());
  app.addHook("onRequest", (_request, reply, done) => {
    for (const [name, value] of headers) reply.raw.setHeader(name, value);
    done();
  });
}
