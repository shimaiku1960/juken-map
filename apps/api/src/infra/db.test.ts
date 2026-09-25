import { afterAll, describe, expect, it } from "vitest";
import { cleanup, createUser } from "../test-db/fixtures.ts";
import { select, selectDateStrings, toIsoString } from "./db.ts";

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

  it("selectDateStrings は DATETIME(3) を文字列のまま返し、toIsoString は Date 経由と同じ形になる", async () => {
    // 画面へ返す一覧は Date を経由しない。これまでの Date → toISOString と
    // 1文字でも違えば画面の日付の比較や並びが変わるので、同じ値を両方の道で取って突き合わせる。
    // 小数0の値（.000）と日付が変わる直前の値も含める。
    const sql = `SELECT CAST(? AS DATETIME(3)) AS value`;
    for (const literal of [
      "2027-02-20 00:00:00.000",
      "2026-09-24 15:00:00.000",
      "2026-12-31 23:59:59.999",
      "2026-09-01 12:34:56.789",
    ]) {
      const [viaDate] = await select<{ value: Date }>(sql, [literal]);
      const [asString] = await selectDateStrings<{ value: Date }>(sql, [literal]);

      expect(asString.value).toBe(literal);
      expect(toIsoString(asString.value)).toBe(viaDate.value.toISOString());
    }
  });

  it("selectDateStrings を使っても、ほかの select は Date のまま返す", async () => {
    // Better Auth も同じプールを使い、日時を Date として受け取る前提。
    // 文字列で返すのは selectDateStrings を呼んだその1本だけでなければならない。
    await selectDateStrings("SELECT CAST('2027-02-20 00:00:00.000' AS DATETIME(3)) AS value");
    const [row] = await select<{ value: unknown }>(
      "SELECT CAST('2027-02-20 00:00:00.000' AS DATETIME(3)) AS value"
    );

    expect(row.value).toBeInstanceOf(Date);
  });
});
