import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import { daysUntil, formatExamDate } from "@/lib/date";

export type CountdownGoal = {
  id: number;
  universityName: string;
  facultyName: string;
  examDate: Date | string;
  isFirstChoice: boolean;
};

// あと日数の表示（当日/過去も自然な日本語にする）
const daysLabel = (n: number) => {
  if (n > 0) return `あと ${n} 日`;
  if (n === 0) return "今日";
  return `${-n} 日前に終了`;
};

// 1枚のカウントダウンカード
const CountdownItem = ({
  label,
  goal,
}: {
  label: string;
  goal: CountdownGoal;
}) => {
  const n = daysUntil(goal.examDate);
  return (
    <Card>
      <CardContent className="py-5">
        <p className="text-sm text-gray-500">{label}</p>
        <p className="font-medium">
          {goal.universityName} {goal.facultyName}
        </p>
        <p className="mt-2 text-3xl font-bold text-blue-600">{daysLabel(n)}</p>
        <p className="mt-1 text-sm text-gray-500">
          {formatExamDate(goal.examDate)}
        </p>
      </CardContent>
    </Card>
  );
};

export default function ExamCountdown({ goals }: { goals: CountdownGoal[] }) {
  if (goals.length === 0) {
    return (
      <Card>
        <CardContent className="py-6 text-center">
          <p className="text-gray-600">まだ志望校が登録されていません。</p>
          <Link
            href="/profile"
            className="text-blue-600 hover:underline text-sm"
          >
            プロフィールで志望校を設定する →
          </Link>
        </CardContent>
      </Card>
    );
  }

  const firstChoice = goals.find((g) => g.isFirstChoice) ?? null;

  // 直近の受験（今日以降で一番近いもの）
  const nextExam =
    goals
      .filter((g) => daysUntil(g.examDate) >= 0)
      .sort((a, b) => daysUntil(a.examDate) - daysUntil(b.examDate))[0] ?? null;

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {firstChoice && <CountdownItem label="第一志望" goal={firstChoice} />}
      {nextExam && nextExam.id !== firstChoice?.id && (
        <CountdownItem label="次の受験" goal={nextExam} />
      )}
    </div>
  );
}
