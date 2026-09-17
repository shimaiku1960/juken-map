import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { developmentTargets, fastifyLoggingOptions } from "@/api/observability/logger";
import { buildTestApp, takeLogLines } from "@/api/test-support";

describe("developmentTargets", () => {
  it("LOG_FILE が無ければ画面（pino-pretty）にだけ出す", () => {
    expect(developmentTargets(undefined).map((t) => t.target)).toEqual(["pino-pretty"]);
  });

  it("LOG_FILE があれば、リポジトリのルートを基準にしたファイルにも JSON を書く", () => {
    const file = developmentTargets("logs/api.log").find((t) => t.target === "pino/file");
    const destination = (file?.options as { destination: string }).destination;
    expect(path.isAbsolute(destination)).toBe(true);
    // apps/api ではなくルート直下の logs/ に書く（.env と同じ基準）。
    expect(destination).toBe(
      path.resolve(import.meta.dirname, "../../../../logs/api.log")
    );
  });
});

describe("reqId の採番", () => {
  const original = process.env.NODE_ENV;
  afterEach(() => {
    process.env.NODE_ENV = original;
  });

  it("本番は UUID（デプロイをまたいでも取り違えない）", () => {
    process.env.NODE_ENV = "production";
    expect(fastifyLoggingOptions.genReqId()).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
    );
  });

  it("開発は短くする（全行の先頭に出るので、長いと内容が押し出される）", () => {
    process.env.NODE_ENV = "development";
    expect(fastifyLoggingOptions.genReqId()).toMatch(/^[0-9a-f]{8}$/);
  });
});

describe("リクエストのログ", () => {
  beforeEach(() => {
    takeLogLines();
  });

  it("完了時に1行、メソッド・パス・ステータス・所要時間・reqId を書き出す", async () => {
    const app = buildTestApp((app) => {
      app.get("/api/example", async () => ({ ok: true }));
    });

    await app.inject({ method: "GET", url: "/api/example?token=secret" });

    const lines = takeLogLines().filter((line) => line.msg === "request completed");
    expect(lines).toEqual([
      expect.objectContaining({
        level: 30,
        // 本番以外は短い形（採番の使い分けは下の「reqId の採番」で確かめる）。
        reqId: expect.stringMatching(/^[0-9a-f]{8}$/),
        req: { method: "GET", url: "/api/example" },
        res: { statusCode: 200 },
        responseTime: expect.any(Number),
      }),
    ]);
    // トークンはどの行にも残らない。
    expect(JSON.stringify(lines)).not.toContain("secret");
  });

  it("ハンドラの想定外の例外（500）を、エラーの中身ごと書き出す", async () => {
    const app = buildTestApp((app) => {
      app.get("/api/broken", async () => {
        throw new Error("boom");
      });
    });

    const res = await app.inject({ method: "GET", url: "/api/broken" });

    expect(res.statusCode).toBe(500);
    const errors = takeLogLines().filter((line) => line.level === 50);
    expect(errors).toContainEqual(
      expect.objectContaining({
        reqId: expect.any(String),
        err: expect.objectContaining({ message: "boom", stack: expect.any(String) }),
      })
    );
  });
});
