import { describe, it, expect } from "vitest";
import { demoReadOnlyGuard, DEMO_EMAIL } from "./demo";

// デモ閲覧専用ガード（各 API ルートの mutation 手前で呼ぶ門番）の単体テスト。
// 外部依存は無く、email の一致だけで挙動が決まる純粋な分岐なのでモック不要。
describe("demoReadOnlyGuard", () => {
  it("デモアカウントなら 403 を返す（編集拒否）", async () => {
    const res = demoReadOnlyGuard({ user: { email: DEMO_EMAIL } });

    expect(res).not.toBeNull();
    expect(res!.status).toBe(403);
    await expect(res!.json()).resolves.toEqual({
      error: "デモアカウントは閲覧専用です",
    });
  });

  it("通常ユーザーなら null を返す（素通し）", () => {
    expect(
      demoReadOnlyGuard({ user: { email: "user@example.com" } })
    ).toBeNull();
  });

  it("session が null でも null を返す（未ログインは各ルートの 401 チェックに任せる）", () => {
    expect(demoReadOnlyGuard(null)).toBeNull();
  });
});
