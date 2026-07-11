import { ymdLocal } from "@/lib/date";

// 集計に必要な最小限の形（date と done だけ見る）
export type StatPlan = {
  date: Date | string;
  done: boolean;
};

// "YYYY-MM-DD" を n 日ずらした "YYYY-MM-DD" を返す（ローカル基準）
function shiftYmd(ymd: string, days: number): string {
  const [y, m, d] = ymd.split("-").map(Number);
  const dt = new Date(y, m - 1, d);
  dt.setDate(dt.getDate() + days);
  return ymdLocal(dt);
}

// その日に done が1件でもある日を「達成日」とし、
// 今日から遡って連続する達成日数を返す。
// 今日がまだ未達成でも、昨日が達成なら昨日基点で数える（当日中は途切れない）。
export function computeStreak(plans: StatPlan[], today: string): number {
  // 達成日（done 1件以上ある日）の集合を作る
  const achievedDays = new Set<string>();
  for (const p of plans) {
    if (p.done) achievedDays.add(ymdLocal(p.date));
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
  count: number; // その日に done した件数
};

// 対象月（anchor が属する月）の日ごとの done 件数を、
// 「週 × 7列（日〜土）」のグリッドとして返す。描画側は二重ループで並べるだけ。
export function computeHeatmap(
  plans: StatPlan[],
  anchor: string
): HeatmapCell[][] {
  // 日付ごとの done 件数を集計
  const counts = new Map<string, number>();
  for (const p of plans) {
    if (!p.done) continue;
    const key = ymdLocal(p.date);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }

  const [year, month] = anchor.split("-").map(Number);
  const firstDay = new Date(year, month - 1, 1);
  const daysInMonth = new Date(year, month, 0).getDate();
  const leadingBlanks = firstDay.getDay(); // 月初の曜日（0=日）ぶんの空白

  // 空白 → 各日、の順にフラットなセル列を作る
  const cells: HeatmapCell[] = [];
  for (let i = 0; i < leadingBlanks; i++) {
    cells.push({ ymd: null, count: 0 });
  }
  for (let d = 1; d <= daysInMonth; d++) {
    const ymd = ymdLocal(new Date(year, month - 1, d));
    cells.push({ ymd, count: counts.get(ymd) ?? 0 });
  }
  // 末尾を7の倍数まで空白で埋める
  while (cells.length % 7 !== 0) {
    cells.push({ ymd: null, count: 0 });
  }

  // 7列ずつの週に分割
  const weeks: HeatmapCell[][] = [];
  for (let i = 0; i < cells.length; i += 7) {
    weeks.push(cells.slice(i, i + 7));
  }
  return weeks;
}
