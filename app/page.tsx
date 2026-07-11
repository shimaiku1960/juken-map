import { auth } from "@/lib/auth";
import { headers } from "next/headers";
import prisma from "@/lib/prisma";
import { redirect } from "next/navigation";
import Link from "next/link";
import ExamCountdown, {
  type CountdownGoal,
} from "@/app/components/ExamCountdown";
import TodayStudyPlans, {
  type TodayPlan,
} from "@/app/components/TodayStudyPlans";
import StreakBadge from "@/app/components/StreakBadge";
import StudyHeatmap from "@/app/components/StudyHeatmap";
import { Card, CardContent } from "@/components/ui/card";
import { ymdLocal, todayYmd, ymdAfterDays } from "@/lib/date";
import { studyPlanLabel } from "@/lib/studyPlan";
import { computeStreak, computeHeatmap } from "@/lib/studyStats";

const Home = async () => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });


  if (!session) {
    redirect("/login");
  }

  const goals = await prisma.finalGoal.findMany({
    where: { userId: session.user.id },
    include: {
      faculty: {
        include: { university: true },
      },
    },
    orderBy: { createdAt: "asc" },
  });

  const countdownGoals: CountdownGoal[] = goals.map((goal) => ({
    id: goal.id,
    universityName: goal.faculty.university.name,
    facultyName: goal.faculty.name,
    examDate: goal.faculty.examDate,
    isFirstChoice: goal.isFirstChoice,
  }));

  // ② 学習予定（今日 / 今後7日間）
  const plans = await prisma.studyPlan.findMany({
    where: { userId: session.user.id },
    orderBy: { date: "asc" },
    include: { textbook: true },
  });

  const todayStr = todayYmd();
  const weekEndStr = ymdAfterDays(7);

  const todayPlans: TodayPlan[] = plans
    .filter((p) => ymdLocal(p.date) === todayStr)
    .map((p) => ({ id: p.id, content: studyPlanLabel(p), done: p.done }));

  const weekCount = plans.filter((p) => {
    const d = ymdLocal(p.date);
    return d >= todayStr && d <= weekEndStr;
  }).length;

  // 連続達成日数（ストリーク）
  const streak = computeStreak(plans, todayStr);

  // 今月の学習ヒートマップ
  const heatmapWeeks = computeHeatmap(plans, todayStr);
  const monthLabel = new Date().toLocaleDateString("ja-JP", {
    year: "numeric",
    month: "long",
  });

  // ③ 志望校サマリー
  const firstChoice = goals.find((g) => g.isFirstChoice) ?? null;
  const otherCount = goals.filter((g) => !g.isFirstChoice).length;

  return (
    <main className="w-full mx-auto max-w-3xl p-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-3xl font-bold">ダッシュボード</h1>
        <StreakBadge streak={streak} />
      </div>

      <section className="mb-8">
        <h2 className="text-xl font-bold mb-3">受験カウントダウン</h2>
        <ExamCountdown goals={countdownGoals} />
      </section>

      <section className="mb-8">
        <h2 className="text-xl font-bold mb-3">今日の学習予定</h2>
        <TodayStudyPlans plans={todayPlans} weekCount={weekCount} />
      </section>

      <section className="mb-8">
        <h2 className="text-xl font-bold mb-3">学習の記録</h2>
        <Card>
          <CardContent className="py-5">
            <StudyHeatmap weeks={heatmapWeeks} monthLabel={monthLabel} />
          </CardContent>
        </Card>
      </section>

      <section className="mb-8">
        <h2 className="text-xl font-bold mb-3">志望校</h2>
        <Card>
          <CardContent className="py-5">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-500">第一志望</p>
                <p className="font-medium">
                  {firstChoice
                    ? `${firstChoice.faculty.university.name} ${firstChoice.faculty.name}`
                    : "未設定"}
                </p>
                <p className="mt-1 text-sm text-gray-500">
                  併願：{otherCount} 校
                </p>
              </div>
              <Link
                href="/profile"
                className="text-blue-600 hover:underline text-sm"
              >
                志望校を設定 →
              </Link>
            </div>
          </CardContent>
        </Card>
      </section>

      <p className="text-sm text-gray-500">
        ※ 表示している受験日は暫定です。正式な日程は各大学の募集要項で必ずご確認ください。
      </p>
    </main>
  );
};

export default Home;