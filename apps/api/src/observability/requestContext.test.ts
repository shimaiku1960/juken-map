import { describe, expect, it } from "vitest";
import { measured } from "@/api/observability/measured";
import { currentReqId } from "@/api/observability/requestContext";
import { buildTestApp, request, takeLogLines } from "@/api/test-support";

/**
 * 狙いは「measured() が reqId を書く」ことではなく、
 * **リクエストの処理の奥まで文脈が届く**ことの確認。
 *
 * AsyncLocalStorage は非同期の境界を越えると簡単に失われる。Fastify のフックで
 * done() を包む今のやり方が本当に効いているかは、実際にリクエストを流して
 * 奥で読んだ reqId が手前の行と一致するかを見ないと分からない。
 */
describe("リクエストごとの文脈", () => {
  it("ハンドラの奥で呼んだ measured() に、そのリクエストの reqId が付く", async () => {
    const app = buildTestApp((app) => {
      app.get("/probe", async () => {
        // ルート → サービス → さらに奥、を模して非同期を数段はさむ。
        await new Promise((resolve) => setTimeout(resolve, 1));
        await measured("probe.deep", async () => {
          await new Promise((resolve) => setTimeout(resolve, 1));
        });
        return { ok: true };
      });
    });

    takeLogLines();
    const response = await request(app, "GET", "/probe");
    expect(response.statusCode).toBe(200);

    const lines = takeLogLines();
    const measuredLine = lines.find((line) => line.operation === "probe.deep");
    const completedLine = lines.find((line) => line.msg === "request completed");

    expect(measuredLine?.reqId).toBeDefined();
    // 手前（Fastify が出す行）と奥（measured）が同じ id で束ねられている。
    expect(measuredLine?.reqId).toBe(completedLine?.reqId);
  });

  it("別々のリクエストの行が、違う reqId で区別できる", async () => {
    const app = buildTestApp((app) => {
      app.get("/probe/:name", async (request) => {
        const { name } = request.params as { name: string };
        // 2本を同時に走らせ、待ち時間をずらして処理を交錯させる。
        await new Promise((resolve) => setTimeout(resolve, name === "slow" ? 20 : 1));
        await measured(`probe.${name}`, async () => {});
        return { ok: true };
      });
    });

    takeLogLines();
    await Promise.all([
      request(app, "GET", "/probe/slow"),
      request(app, "GET", "/probe/fast"),
    ]);

    const lines = takeLogLines();
    const slow = lines.find((line) => line.operation === "probe.slow");
    const fast = lines.find((line) => line.operation === "probe.fast");

    expect(slow?.reqId).toBeDefined();
    expect(fast?.reqId).toBeDefined();
    // ここが本題。混ざっていると同じ id になる。
    expect(slow?.reqId).not.toBe(fast?.reqId);
  });

  it("リクエストの外では undefined を返す（cron や seed から呼ばれる場合）", () => {
    expect(currentReqId()).toBeUndefined();
  });
});
