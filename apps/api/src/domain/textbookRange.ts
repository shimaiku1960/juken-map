/**
 * 実績・予定の「学習範囲」が、参考書の逆算設定と噛み合っているかを判定する。
 *
 * DB も HTTP も知らない純粋な規則。実績の新規作成と編集の両方から呼ぶ。
 * 問題なければ null、あればユーザー向けの文言を返す。
 */
export function textbookRangeError(
  textbook: { rangeUnit: string | null; totalAmount: number | null },
  data: { rangeEnd?: number | null; rangeUnit?: string | null }
) {
  if (data.rangeEnd == null) return null;

  if (textbook.rangeUnit != null && data.rangeUnit !== textbook.rangeUnit) {
    return "範囲の単位を参考書の逆算設定に合わせてください";
  }

  if (textbook.totalAmount != null && data.rangeEnd > textbook.totalAmount) {
    return `終了位置は参考書の総量（${textbook.totalAmount}）以下にしてください`;
  }

  return null;
}
