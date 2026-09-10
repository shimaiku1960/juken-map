import { describe, it, expect, vi } from "vitest";

// betterAuth の初期化を走らせないため、auth モジュールだけ差し替える。
vi.mock("./auth.ts", () => ({ auth: { api: { getSession: vi.fn() } } }));

const { denyDemoWrite } = await import("./context.ts");
const { DEMO_EMAIL } = await import("@/shared/demo");

// デモ閲覧専用ガード（編集系ハンドラの手前で呼ぶ門番）の単体テスト。
// 外部依存は無く email の一致だけで挙動が決まるので、reply だけ最小限に作る。
function fakeReply() {
  const sent: { status?: number; body?: unknown } = {};
  const reply = {
    code(status: number) {
      sent.status = status;
      return reply;
    },
    send(body: unknown) {
      sent.body = body;
      return reply;
    },
  };
  return { reply, sent };
}

describe("denyDemoWrite", () => {
  it("デモアカウントなら 403 を送って true を返す（編集拒否）", () => {
    const { reply, sent } = fakeReply();

    const stopped = denyDemoWrite(
      { user: { email: DEMO_EMAIL } },
      reply as never
    );

    expect(stopped).toBe(true);
    expect(sent.status).toBe(403);
    expect(sent.body).toEqual({ error: "デモアカウントは閲覧専用です" });
  });

  it("通常ユーザーなら何も送らず false を返す（素通し）", () => {
    const { reply, sent } = fakeReply();

    const stopped = denyDemoWrite(
      { user: { email: "user@example.com" } },
      reply as never
    );

    expect(stopped).toBe(false);
    expect(sent.status).toBeUndefined();
  });
});
