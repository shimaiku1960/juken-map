import { createHash } from "node:crypto";
import Fastify from "fastify";
import { afterEach, describe, expect, it, vi } from "vitest";

// seo.ts は microCMS のクライアントを読み込み時に作る（CI には接続情報が無い）。
vi.mock("@/api/infra/microcms", () => ({ getBlog: vi.fn(), listBlogs: vi.fn() }));

const { contentSecurityPolicy, registerSecurityHeaders } = await import("./security-headers.ts");
const { injectMeta } = await import("./seo.ts");

afterEach(() => {
  vi.unstubAllEnvs();
});

function appWithHeaders() {
  const app = Fastify();
  registerSecurityHeaders(app);
  app.get("/api/normal", async () => ({ ok: true }));
  // Better Auth と同じく、Fastify の応答処理を通さず生の応答へ直接書くルート。
  app.get("/api/auth/hijacked", async (_request, reply) => {
    reply.hijack();
    reply.raw.writeHead(200, { "content-type": "application/json" });
    reply.raw.end("{}");
  });
  return app;
}

describe("registerSecurityHeaders", () => {
  it.each(["/api/normal", "/api/auth/hijacked"])("%s の応答に付く", async (url) => {
    const res = await appWithHeaders().inject({ method: "GET", url });

    expect(res.statusCode).toBe(200);
    expect(res.headers["strict-transport-security"]).toBe("max-age=31536000");
    expect(res.headers["x-content-type-options"]).toBe("nosniff");
    expect(res.headers["x-frame-options"]).toBe("DENY");
    expect(res.headers["referrer-policy"]).toBe("strict-origin-when-cross-origin");
    // まだ止めない（報告だけ）。
    expect(res.headers["content-security-policy"]).toBeUndefined();
    expect(res.headers["content-security-policy-report-only"]).toContain("default-src 'self'");
  });
});

describe("contentSecurityPolicy", () => {
  it("Faro の送り先はオリジンだけを許す。設定が無ければ入れない", () => {
    expect(
      contentSecurityPolicy({ FARO_COLLECTOR_URL: "https://faro.example.net/collect/key" })
    ).toMatch(/connect-src 'self' https:\/\/faro\.example\.net /);
    expect(contentSecurityPolicy({})).not.toContain("faro");
  });

  it("GA4 のインラインスクリプトを、実際に差し込む中身のハッシュで許す", () => {
    vi.stubEnv("GA_MEASUREMENT_ID", "G-TEST123");
    const html = injectMeta("<html><head><title>x</title></head></html>", {
      title: "t",
      description: "d",
      ogTitle: "t",
      ogType: "website",
      ogImage: "https://juken-map.com/og.png",
      noindex: false,
    });
    const inline = /<script>([\s\S]*?)<\/script>/.exec(html)![1]!;
    const hash = createHash("sha256").update(inline).digest("base64");

    const scriptSrc = /script-src ([^;]*)/.exec(contentSecurityPolicy())![1]!;
    expect(scriptSrc).toContain(`'sha256-${hash}'`);
    // スクリプトはインラインを丸ごと許さない（スタイルは Sonner のために許している）。
    expect(scriptSrc).not.toContain("'unsafe-inline'");
  });
});
