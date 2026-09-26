import { describe, expect, it } from "vitest";
import { BODY_LIMIT } from "@/api/error-handling";
import { buildTestApp, publicRoute, request } from "@/api/test-support";

// 想定外の例外・大きすぎるボディ・reqId の3点を、本番と同じ組み立て（buildTestApp）で確かめる。
function app() {
  return buildTestApp((app) => {
    app.get("/api/boom", publicRoute, async () => {
      // 5xx の本文へ漏れてはいけない文字列。実際に漏れていたのは err.message だった。
      throw new Error("SELECT * FROM user WHERE email = 'secret@example.com'");
    });
    app.post("/api/echo", publicRoute, async (request) => ({ received: request.body !== undefined }));
  });
}

describe("エラー応答", () => {
  it("想定外の例外は 500 の固定文言にして、内部の情報を返さない", async () => {
    const res = await request(app(), "GET", "/api/boom");

    expect(res.statusCode).toBe(500);
    expect(res.payload).not.toContain("SELECT");
    expect(res.payload).not.toContain("secret@example.com");

    const body = res.json();
    expect(body.error).toBe("サーバー側で問題が発生しました");
    expect(body.code).toBe("INTERNAL_ERROR");
    expect(body.reqId).toBeTruthy();
  });

  it("応答ヘッダーの x-request-id と本文の reqId が一致する（ログと突き合わせるため）", async () => {
    const res = await request(app(), "GET", "/api/boom");

    expect(res.headers["x-request-id"]).toBe(res.json().reqId);
  });

  it("上限を超えたボディはハンドラへ入る前に 413 で止まる", async () => {
    const res = await app().inject({
      method: "POST",
      url: "/api/echo",
      payload: JSON.stringify({ value: "a".repeat(BODY_LIMIT) }),
      headers: { "content-type": "application/json" },
    });

    expect(res.statusCode).toBe(413);
    expect(res.json()).toMatchObject({
      error: "送信されたデータが大きすぎます",
      code: "FST_ERR_CTP_BODY_TOO_LARGE",
    });
  });

  it("上限内のボディはハンドラへ届く", async () => {
    const res = await app().inject({
      method: "POST",
      url: "/api/echo",
      payload: JSON.stringify({ value: "a".repeat(1000) }),
      headers: { "content-type": "application/json" },
    });

    expect(res.statusCode).toBe(200);
    expect(res.json()).toEqual({ received: true });
  });

  // /api/* の 404 を errorBody の形で返すのは server.ts の setNotFoundHandler で、
  // ここ（ルート単位の組み立て）には載っていない。ヘッダーだけ確認する。
  it("エラー応答には x-request-id が付く", async () => {
    const res = await request(app(), "GET", "/api/does-not-exist");

    expect(res.statusCode).toBe(404);
    expect(res.headers["x-request-id"]).toBeTruthy();
  });
});
