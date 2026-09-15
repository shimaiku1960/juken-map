import { beforeEach, describe, expect, it } from "vitest";
import { pathForLog } from "@/api/observability/logger";
import { buildTestApp, takeLogLines } from "@/api/test-support";

describe("pathForLog", () => {
  it("? 以降（トークンや OAuth の code）を落とす", () => {
    expect(pathForLog("/api/auth/verify-email?token=secret&callbackURL=%2F")).toBe(
      "/api/auth/verify-email"
    );
    expect(pathForLog("/api/line/oauth/callback?state=s&code=c")).toBe(
      "/api/line/oauth/callback"
    );
  });

  it("パスに入るパスワード再設定のトークンを伏せる", () => {
    expect(pathForLog("/api/auth/reset-password/secret-token?callbackURL=%2F")).toBe(
      "/api/auth/reset-password/:token"
    );
  });

  it("トークンを含まないパスはそのまま残す", () => {
    expect(pathForLog("/api/study-logs/12")).toBe("/api/study-logs/12");
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
        reqId: expect.stringMatching(/^[0-9a-f-]{36}$/),
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
