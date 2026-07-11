import { ymdLocal } from "@/lib/date";
import { SUBJECTS } from "@/lib/subjects";

// 集計に必要な最小限の形（実績＝日付・学習時間・科目だけ見る）
export type StatLog = {
  date: Date | string;
  minutes: number;
  subject?: string | null;
};

// "YYYY-MM-DD" を n 日ずらした "YYYY-MM-DD" を返す（ローカル基準）
function shiftYmd(ymd: string, days: number): string {
  const [y, m, d] = ymd.split("-").map(Number);
  const dt = new Date(y, m - 1, d);
  dt.setDate(dt.getDate() + days);
  return ymdLocal(dt);
}

// その日に学習時間が1分でもある日を「達成日」とし、
// 今日から遡って連続する達成日数を返す。
// 今日がまだ未記録でも、昨日が達成なら昨日基点で数える（当日中は途切れない）。
export function computeStreak(logs: StatLog[], today: string): number {
  // 達成日（合計 minutes が1分以上ある日）の集合を作る
  const achievedDays = new Set<string>();
  for (const l of logs) {
    if (l.minutes > 0) achievedDays.add(ymdLocal(l.date));
  }

  // 起点：今日が達成済みなら今日、そうでなければ昨日から数え始める
  const start = achievedDays.has(today) ? today : shiftYmd(today, -1);

  let streak = 0;
  let cursor = start;
  while (achievedDays.has(cursor)) {
    streak++;
    cursor = shiftYmd(cursor, -1);
  }
  return streak;
}

// ヒートマップの1マス。日付が null のマスは月頭の曜日合わせ用の空白。
export type HeatmapCell = {
  ymd: string | null;
  minutes: number; // その日の合計学習時間（分）
};

// 対象月（anchor が属する月）の日ごとの合計学習時間（分）を、
// 「週 × 7列（日〜土）」のグリッドとして返す。描画側は二重ループで並べるだけ。
export function computeHeatmap(
  logs: StatLog[],
  anchor: string
): HeatmapCell[][] {
  // 日付ごとの合計分を集計
  const totals = new Map<string, number>();
  for (const l of logs) {
    if (l.minutes <= 0) continue;
    const key = ymdLocal(l.date);
    totals.set(key, (totals.get(key) ?? 0) + l.minutes);
  }

  const [year, month] = anchor.split("-").map(Number);
  const firstDay = new Date(year, month - 1, 1);
  const daysInMonth = new Date(year, month, 0).getDate();
  const leadingBlanks = firstDay.getDay(); // 月初の曜日（0=日）ぶんの空白

  // 空白 → 各日、の順にフラットなセル列を作る
  const cells: HeatmapCell[] = [];
  for (let i = 0; i < leadingBlanks; i++) {
    cells.push({ ymd: null, minutes: 0 });
  }
  for (let d = 1; d <= daysInMonth; d++) {
    const ymd = ymdLocal(new Date(year, month - 1, d));
    cells.push({ ymd, minutes: totals.get(ymd) ?? 0 });
  }
  // 末尾を7の倍数まで空白で埋める
  while (cells.length % 7 !== 0) {
    cells.push({ ymd: null, minutes: 0 });
  }

  // 7列ずつの週に分割
  const weeks: HeatmapCell[][] = [];
  for (let i = 0; i < cells.length; i += 7) {
    weeks.push(cells.slice(i, i + 7));
  }
  return weeks;
}

// 科目別の合計学習時間（分）。SUBJECTS の並び・色つきで返す。
export type SubjectMinutes = {
  value: string;
  label: string;
  color: string;
  minutes: number;
};

// 期間 [from, to]（"YYYY-MM-DD" 両端含む）の科目別合計分。
// 科目未設定（null）は「その他(other)」に合算する。
export function computeSubjectMinutes(
  logs: StatLog[],
  from: string,
  to: string
): SubjectMinutes[] {
  const totals = new Map<string, number>();
  for (const l of logs) {
    if (l.minutes <= 0) continue;
    const d = ymdLocal(l.date);
    if (d < from || d > to) continue;
    const key = l.subject ?? "other";
    totals.set(key, (totals.get(key) ?? 0) + l.minutes);
  }

  return SUBJECTS.map((s) => ({
    value: s.value,
    label: s.label,
    color: s.color,
    minutes: totals.get(s.value) ?? 0,
  }));
}
