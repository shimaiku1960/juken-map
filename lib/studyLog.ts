import { RANGE_UNITS } from "@/lib/validations/studyPlan";
import { subjectLabel } from "@/lib/subjects";

// 分 → 「1時間30分」「45分」のような表示に整形する。
export function formatMinutes(total: number): string {
  if (total <= 0) return "0分";
  const h = Math.floor(total / 60);
  const m = total % 60;
  if (h === 0) return `${m}分`;
  if (m === 0) return `${h}時間`;
  return `${h}時間${m}分`;
}

// 単位の値（page/question/…）→ ラベル（ページ/問題/…）
const unitLabel = (value: string | null | undefined) =>
  (value && RANGE_UNITS.find((u) => u.value === value)?.label) || "";

// 実績1件を「科目・時間（＋参考書・範囲）」の表示用テキストにまとめる。
// 例: 英語・1時間30分（青チャート 10〜20ページ）
export function studyLogLabel(log: {
  subject: string | null;
  minutes: number;
  textbook: { name: string } | null;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
}): string {
  const head = `${subjectLabel(log.subject)}・${formatMinutes(log.minutes)}`;

  const detail: string[] = [];
  if (log.textbook) detail.push(log.textbook.name);
  if (log.rangeStart != null && log.rangeEnd != null) {
    detail.push(`${log.rangeStart}〜${log.rangeEnd}${unitLabel(log.rangeUnit)}`);
  }

  return detail.length ? `${head}（${detail.join(" ")}）` : head;
}
