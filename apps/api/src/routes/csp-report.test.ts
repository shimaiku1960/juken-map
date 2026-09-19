import { beforeEach, describe, expect, it, vi } from "vitest";
import { buildTestApp, takeLogLines } from "@/api/test-support";

// security-headers.ts 経由で seo.ts が microCMS のクライアントを作るので差し替える。
vi.mock("@/api/infra/microcms", () => ({ getBlog: vi.fn(), listBlogs: vi.fn() }));

const { parseCspReports, registerCspReportRoutes } = await import("@/api/routes/csp-report");

function app() {
  return buildTestApp((app) => registerCspReportRoutes(app));
}

function post(body: string, contentType: string) {
  return app().inject({
    method: "POST",
    url: "/api/csp-report",
    payload: body,
    headers: { "content-type": contentType },
  });
}

describe("POST /api/csp-report", () => {
  beforeEach(() => {
    takeLogLines();
  });

  it("report-uri の形（Safari・Firefox）を受けてログに残す", async () => {
    const res = await post(
      JSON.stringify({
        "csp-report": {
          "document-uri": "https://juken-map.com/reset-password?token=secret",
          "effective-directive": "style-src-elem",
          "blocked-uri": "inline",
          "line-number": 3,
        },
      }),
      "application/csp-report"
    );

    expect(res.statusCode).toBe(204);
    const logs = takeLogLines().filter((line) => line.msg === "csp violation");
    expect(logs).toHaveLength(1);
    expect(logs[0]!.csp).toEqual({
      documentUrl: "https://juken-map.com/reset-password",
      directive: "style-src-elem",
      blockedUrl: "inline",
      lineNumber: 3,
    });
  });

  it("report-to の形（Chrome）を受け、トークンを伏せてログに残す", async () => {
    const res = await post(
      JSON.stringify([
        {
          type: "csp-violation",
          body: {
            documentURL: "https://juken-map.com/api/auth/reset-password/tok123?callbackURL=x",
            effectiveDirective: "img-src",
            blockedURL: "https://evil.example/pixel.gif?leak=1",
            disposition: "report",
          },
        },
        { type: "deprecation", body: { id: "x" } },
      ]),
      "application/reports+json"
    );

    expect(res.statusCode).toBe(204);
    const logs = takeLogLines().filter((line) => line.msg === "csp violation");
    expect(logs).toHaveLength(1);
    expect(logs[0]!.csp).toMatchObject({
      documentUrl: "https://juken-map.com/api/auth/reset-password/:token",
      directive: "img-src",
      blockedUrl: "https://evil.example/pixel.gif",
      disposition: "report",
    });
  });

  it("壊れた本文でも 204 を返し、何も残さない", async () => {
    const res = await post("{not json", "application/csp-report");

    expect(res.statusCode).toBe(204);
    expect(takeLogLines().filter((line) => line.msg === "csp violation")).toHaveLength(0);
  });
});

describe("parseCspReports", () => {
  it("一度に読む報告は20件まで", () => {
    const many = Array.from({ length: 50 }, () => ({
      type: "csp-violation",
      body: { effectiveDirective: "img-src" },
    }));
    expect(parseCspReports(many)).toHaveLength(20);
  });
});
