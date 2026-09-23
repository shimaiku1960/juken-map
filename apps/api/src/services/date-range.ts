/**
 * 「自分のものを、ある期間ぶんだけ」取るための SQL 条件。実績と予定で同じ形なのでここに置く。
 *
 * 期間を必須にしているのは、実績・予定の一覧がどちらも全期間・全件を返していて、
 * 使い込んだ利用者で応答が数百KBになっていたため（2026-09-21の限界点試験で、
 * アプリ全体の throughput を決めているのがこの応答の大きさだと分かった）。
 */
export type DateRange = { from: string; to?: string };

/**
 * 期間を DATETIME の比較に使える半開区間 [from, toExclusive) にする。
 *
 * 実績と予定の date は「その日の 00:00 UTC」で入っている（infra/db.ts が timezone: "Z" で、
 * 記録時に new Date("YYYY-MM-DD") を渡しているため）。to の当日ぶんを含めたいので、
 * 上限は to の翌日の 00:00 にする。
 *
 * alias は SQL に直接埋める。値ではなく識別子なので ? では渡せない。呼び出し側が渡すのは
 * 自分のクエリの中で書いた固定の別名だけで、利用者の入力は入らない。
 */
export function userDateConditions(
  alias: string,
  userId: string,
  range: DateRange
) {
  const conditions = [`${alias}.userId = ?`, `${alias}.date >= ?`];
  const params: unknown[] = [userId, new Date(range.from)];
  if (range.to !== undefined) {
    conditions.push(`${alias}.date < ?`);
    const toExclusive = new Date(range.to);
    toExclusive.setUTCDate(toExclusive.getUTCDate() + 1);
    params.push(toExclusive);
  }
  return { where: conditions.join(" AND "), params };
}
