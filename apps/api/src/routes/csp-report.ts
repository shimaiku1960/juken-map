import type { FastifyInstance } from "fastify";
import { redactPath } from "../observability/redact.ts";
import { CSP_REPORT_PATH } from "../security-headers.ts";

// ブラウザが送ってくる CSP の違反の報告を、ログ（本番は Grafana の Loki）に残す。
// CSP は止めるモードなので、ここに出たものは「実際にブラウザが読み込みを拒んだ箇所」になる。
// 正当な読み込みが出ていたら、security-headers.ts の許可の一覧に漏れがある。
//
// 報告の形は2種類ある。
// - report-uri: application/csp-report、{"csp-report": {"document-uri": ...}}（今使っているのはこちら）
// - report-to（Reporting API）: application/reports+json、[{"type": "csp-violation", "body": {"documentURL": ...}}]
//   今は出していないが（理由は security-headers.ts）、切り替えてもそのまま受けられるようにしておく。

type Violation = {
  documentUrl?: string;
  directive?: string;
  blockedUrl?: string;
  sourceFile?: string;
  lineNumber?: number;
  disposition?: string;
};

// 認証なしで誰でも送れる口なので、1回で読む件数と大きさに上限を置く。
const MAX_REPORTS = 20;
const BODY_LIMIT = 16 * 1024;

// ログに残す URL からトークンを取り除く（redact.ts と同じ方針）。
// "inline"・"eval"・"data" のような URL でない値はそのまま残す。
function safeUrl(value: unknown) {
  if (typeof value !== "string") return undefined;
  try {
    const url = new URL(value);
    return url.origin + redactPath(url.pathname);
  } catch {
    return value.slice(0, 200);
  }
}

function toViolation(report: Record<string, unknown>): Violation {
  const pick = (...keys: string[]) => keys.map((key) => report[key]).find((v) => v !== undefined);
  const line = pick("lineNumber", "line-number");
  return {
    documentUrl: safeUrl(pick("documentURL", "document-uri")),
    directive: String(pick("effectiveDirective", "effective-directive", "violated-directive") ?? ""),
    blockedUrl: safeUrl(pick("blockedURL", "blocked-uri")),
    sourceFile: safeUrl(pick("sourceFile", "source-file")),
    lineNumber: typeof line === "number" ? line : undefined,
    disposition: typeof pick("disposition") === "string" ? String(pick("disposition")) : undefined,
  };
}

/** 2種類の形の報告を、同じ形の一覧にそろえる。読めないものは捨てる。 */
export function parseCspReports(body: unknown): Violation[] {
  const isObject = (v: unknown): v is Record<string, unknown> =>
    typeof v === "object" && v !== null && !Array.isArray(v);

  if (Array.isArray(body)) {
    return body
      .filter((item) => isObject(item) && item.type === "csp-violation" && isObject(item.body))
      .slice(0, MAX_REPORTS)
      .map((item) => toViolation((item as { body: Record<string, unknown> }).body));
  }
  if (isObject(body) && isObject(body["csp-report"])) {
    return [toViolation(body["csp-report"])];
  }
  return [];
}

export function registerCspReportRoutes(app: FastifyInstance) {
  app.addContentTypeParser(
    ["application/csp-report", "application/reports+json"],
    { parseAs: "string", bodyLimit: BODY_LIMIT },
    (_req, body, done) => {
      try {
        done(null, JSON.parse(String(body)));
      } catch {
        done(null, undefined);
      }
    }
  );

  app.post(CSP_REPORT_PATH, { bodyLimit: BODY_LIMIT, config: { access: "anonymous-write" } }, async (request, reply) => {
    for (const violation of parseCspReports(request.body)) {
      request.log.warn({ csp: violation }, "csp violation");
    }
    // ブラウザは中身を読まない。報告の形が崩れていても、送り直させないよう常に 204。
    return reply.code(204).send();
  });
}
