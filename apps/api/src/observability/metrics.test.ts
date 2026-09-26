import { beforeEach, describe, expect, it } from "vitest";
import { registerMetrics, registry } from "@/api/observability/metrics";
import { buildTestApp, publicRoute } from "@/api/test-support";

async function requestCount(labels: string) {
  const text = await registry.metrics();
  const line = text
    .split("\n")
    .find((l) => l.startsWith(`http_requests_total{${labels}}`));
  return line ? Number(line.split(" ").at(-1)) : 0;
}

describe("リクエストのメトリクス", () => {
  beforeEach(() => {
    registry.resetMetrics();
  });

  it("実際の URL ではなくルートの型ごとに数える（ID でラベルが増えない）", async () => {
    const app = buildTestApp((app) => {
      registerMetrics(app);
      app.get("/api/items/:id", publicRoute, async () => ({ ok: true }));
    });

    await app.inject({ method: "GET", url: "/api/items/1" });
    await app.inject({ method: "GET", url: "/api/items/2?token=secret" });

    expect(
      await requestCount('method="GET",route="/api/items/:id",status_code="200"')
    ).toBe(2);
    const text = await registry.metrics();
    expect(text).not.toContain("/api/items/1");
    expect(text).not.toContain("secret");
  });

  it("想定外の例外（500）もステータスコード付きで数える", async () => {
    const app = buildTestApp((app) => {
      registerMetrics(app);
      app.get("/api/broken", publicRoute, async () => {
        throw new Error("boom");
      });
    });

    await app.inject({ method: "GET", url: "/api/broken" });

    expect(
      await requestCount('method="GET",route="/api/broken",status_code="500"')
    ).toBe(1);
    // 所要時間も同じラベルで1件記録される。
    expect(await registry.metrics()).toContain(
      'http_request_duration_seconds_count{method="GET",route="/api/broken",status_code="500"} 1'
    );
  });

  it("onRequest で横取りする Better Auth のリクエストも数える", async () => {
    const app = buildTestApp((app) => {
      registerMetrics(app);
      // server.ts と同じく、ルートを登録せずに生の応答へ直接書く。
      app.addHook("onRequest", async (request, reply) => {
        if (!request.url.startsWith("/api/auth/")) return;
        reply.hijack();
        reply.raw.writeHead(401).end();
      });
    });

    await app.inject({ method: "POST", url: "/api/auth/sign-in/email" });

    expect(
      await requestCount('method="POST",route="/api/auth/*",status_code="401"')
    ).toBe(1);
  });

  it("画面と存在しない API は、それぞれ1つのラベルにまとめる", async () => {
    const app = buildTestApp((app) => {
      registerMetrics(app);
    });

    await app.inject({ method: "GET", url: "/dashboard" });
    await app.inject({ method: "GET", url: "/assets/index-abc123.js" });
    await app.inject({ method: "GET", url: "/api/nope" });

    expect(
      await requestCount('method="GET",route="(web)",status_code="404"')
    ).toBe(2);
    expect(
      await requestCount('method="GET",route="(unmatched)",status_code="404"')
    ).toBe(1);
  });
});
