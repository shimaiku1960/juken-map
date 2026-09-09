// 受験日程タイムライン用のロジック。確定した受験校（decided）の入試日を
// 受験日順に並べ、同日受験・連戦（前後どちらかの日にも別の受験がある）を検出する。
// UI から切り離した純粋関数にしてテストしやすくする。

export type ExamScheduleInput = {
  id: number;
  universityName: string;
  facultyName: string;
  examDate: string | Date;
  isFirstChoice: boolean;
  tags: string[];
};

export type ExamScheduleEntry = ExamScheduleInput & {
  // 同じ暦日にある「他の」受験の数（0 なら重複なし、1 以上で同日受験の警告）
  sameDayCount: number;
  // 前日または翌日にも別の受験があるか（連戦の警告）
  backToBack: boolean;
};

// ローカル暦日を「エポックからの日数」に変換する。時刻・タイムゾーンの
// ぶれを除き、日単位の比較（同日・前後日）を安全にするためのキー。
const toDayNumber = (value: string | Date): number => {
  const date = new Date(value);
  const localMidnight = new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate()
  );
  return Math.floor(localMidnight.getTime() / 86_400_000);
};

export function buildExamSchedule(
  entries: ExamScheduleInput[]
): ExamScheduleEntry[] {
  const dayNumbers = entries.map((entry) => toDayNumber(entry.examDate));

  // 暦日ごとの件数と、存在する暦日の集合を作る
  const countByDay = new Map<number, number>();
  const daySet = new Set<number>();
  dayNumbers.forEach((day) => {
    countByDay.set(day, (countByDay.get(day) ?? 0) + 1);
    daySet.add(day);
  });

  const result = entries.map((entry, index) => {
    const day = dayNumbers[index];
    return {
      ...entry,
      sameDayCount: (countByDay.get(day) ?? 1) - 1,
      backToBack: daySet.has(day - 1) || daySet.has(day + 1),
    };
  });

  // 受験日の早い順に並べる（俯瞰しやすく）
  result.sort((a, b) => toDayNumber(a.examDate) - toDayNumber(b.examDate));
  return result;
}
