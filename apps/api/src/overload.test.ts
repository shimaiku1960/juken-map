import { describe, expect, it } from "vitest";
import { registerOverloadProtection } from "@/api/overload";
import { buildTestApp, request } from "@/api/test-support";

// 上限2で、ハンドラを止めておける app を作る。release() を呼ぶまで /api/slow は返らない。
function app() {
  let releaseAll = () => {};
  const blocked = new Promise<void>((resolve) => {
    releaseAll = resolve;
  });
  const instance = buildTestApp((app) => {
    registerOverloadProtection(app, 2);
    app.get("/api/slow", async () => {
      await blocked;
      return { ok: true };
    });
    app.get("/api/fast", async () => ({ ok: true }));
    app.get("/api/health", async () => ({ ok: true }));
    app.get("/page", async () => "html");
  });
  return { app: instance, release: () => releaseAll() };
}

describe("過負荷防御", () => {
  it("上限を超えた API は待たせずに 503 と Retry-After で断る", async () => {
    const { app: server, release } = app();
    const inFlight = [request(server, "GET", "/api/slow"), request(server, "GET", "/api/slow")];
    // 2本が処理中に入るまで待つ。
    await new Promise((resolve) => setImmediate(resolve));

    const res = await request(server, "GET", "/api/fast");

    expect(res.statusCode).toBe(503);
    expect(res.headers["retry-after"]).toBe("1");
    expect(res.json()).toMatchObject({
      code: "OVERLOADED",
      error: "ただいま混み合っています。少し待ってからもう一度お試しください",
    });

    release();
    const done = await Promise.all(inFlight);
    expect(done.map((r) => r.statusCode)).toEqual([200, 200]);
  });

  it("処理中の分が返れば、また受け付ける", async () => {
    const { app: server, release } = app();
    const inFlight = [request(server, "GET", "/api/slow"), request(server, "GET", "/api/slow")];
    await new Promise((resolve) => setImmediate(resolve));
    release();
    await Promise.all(inFlight);

    const res = await request(server, "GET", "/api/fast");

    expect(res.statusCode).toBe(200);
  });

  it("死活監視と画面は混雑中でも断らない", async () => {
    const { app: server, release } = app();
    const inFlight = [request(server, "GET", "/api/slow"), request(server, "GET", "/api/slow")];
    await new Promise((resolve) => setImmediate(resolve));

    expect((await request(server, "GET", "/api/health")).statusCode).toBe(200);
    expect((await request(server, "GET", "/page")).statusCode).toBe(200);

    release();
    await Promise.all(inFlight);
  });
});
