import { RANGE_UNITS } from "@/lib/validations/studyPlan";

// 単位の値（page/question/chapter）→ ラベル（ページ/問題/章）
const unitLabel = (value: string | null | undefined) =>
  (value && RANGE_UNITS.find((u) => u.value === value)?.label) || "";

// 学習予定1件を「参考書＋範囲（＋メモ）」の表示用1行にまとめる。
// 例: 青チャートIA 12〜45ページ（文法だけ）
export function studyPlanLabel(plan: {
  textbook: { name: string } | null;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  content: string | null;
}): string {
  const parts: string[] = [];

  if (plan.textbook) parts.push(plan.textbook.name);

  if (plan.rangeStart != null && plan.rangeEnd != null) {
    parts.push(`${plan.rangeStart}〜${plan.rangeEnd}${unitLabel(plan.rangeUnit)}`);
  }

  let label = parts.join(" ");

  if (plan.content) {
    label = label ? `${label}（${plan.content}）` : plan.content;
  }

  return label || "（内容なし）";
}
