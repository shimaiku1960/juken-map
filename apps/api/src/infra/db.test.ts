import { afterAll, describe, expect, it } from "vitest";
import { cleanup, createUser } from "../test-db/fixtures.ts";
import { select } from "./db.ts";

// ドライバの設定そのものを確かめる。
//
// アプリの書き込み→読み込みを往復させるだけのテストでは、時間帯を間違えても
// 往復でずれが打ち消されて通ってしまう。Prisma が書いてきた既存の行や、
// 別の接続（Better Auth など）とずれるのが実害なので、DB 側の生の文字列と突き合わせる。

afterAll(cleanup);

describe("infra/db の接続設定", () => {
  it("Date は UTC の日時として DB へ渡る", async () => {
    const [row] = await select<{ sent: string }>(
      "SELECT DATE_FORMAT(CAST(? AS DATETIME(3)), '%Y-%m-%d %H:%i:%s.%f') AS sent",
      [new Date("2027-02-20T00:00:00.000Z")]
    );

    expect(row.sent).toBe("2027-02-20 00:00:00.000000");
  });

  it("DB の DATETIME は UTC として Date に戻る", async () => {
    const [row] = await select<{ value: Date }>(
      "SELECT CAST('2027-02-20 00:00:00.000' AS DATETIME(3)) AS value"
    );

    expect(row.value.toISOString()).toBe("2027-02-20T00:00:00.000Z");
  });

  it("BOOLEAN（TINYINT(1)）の列は 1 / 0 ではなく true / false で返る", async () => {
    // 型情報（TINYINT(1)）は実テーブルの列にしか付かないので、実際の行で確かめる
    const { id } = await createUser();
    const [row] = await select<{ emailVerified: unknown }>(
      "SELECT emailVerified FROM `user` WHERE id = ?",
      [id]
    );

    expect(row.emailVerified).toBe(false);
  });
});
