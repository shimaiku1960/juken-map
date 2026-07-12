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
import StudyRecordDashboard from "@/app/components/StudyRecordDashboard";
import type { StudyLog } from "@/app/hooks/useStudyLogs";
import { Card, CardContent } from "@/components/ui/card";
import { ymdLocal, todayYmd, ymdAfterDays } from "@/lib/date";
import { studyPlanLabel } from "@/lib/studyPlan";

const Home = async () => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    redirect("/login");
  }

  const [goals, plans, logsRaw, textbooks] = await Promise.all([
    prisma.finalGoal.findMany({
      where: { userId: session.user.id },
      include: {
        faculty: {
          include: { university: true },
        },
      },
      orderBy: { createdAt: "asc" },
    }),
    prisma.studyPlan.findMany({
      where: { userId: session.user.id },
      orderBy: { date: "asc" },
      include: { textbook: true },
    }),
    prisma.studyLog.findMany({
      where: { userId: session.user.id },
      orderBy: { date: "desc" },
      include: { textbook: true },
    }),
    prisma.textbook.findMany({
      where: { userId: session.user.id },
      orderBy: { name: "asc" },
    }),
  ]);

  // 受験校（decided）のみをダッシュボードの対象にする。候補（candidate）は志望校ページで比較検討中。
  const decidedGoals = goals.filter((goal) => goal.status === "decided");

  const countdownGoals: CountdownGoal[] = decidedGoals.map((goal) => ({
    id: goal.id,
    universityName: goal.faculty.university.name,
    facultyName: goal.faculty.name,
    examDate: goal.faculty.examDate,
    isFirstChoice: goal.isFirstChoice,
  }));

  // ② 学習予定（今日 / 今後7日間）
  const todayStr = todayYmd();
  const weekEndStr = ymdAfterDays(7);

  const todayPlans: TodayPlan[] = plans
    .filter((p) => ymdLocal(p.date) === todayStr)
    .map((p) => ({ id: p.id, content: studyPlanLabel(p), done: p.done }));

  const weekCount = plans.filter((p) => {
    const d = ymdLocal(p.date);
    return d >= todayStr && d <= weekEndStr;
  }).length;

  // ③ 学習実績（ストリーク・ヒートマップ・科目別はクライアントで集計＝記録すると即反映）
  const initialLogs: StudyLog[] = logsRaw.map((l) => ({
    id: l.id,
    userId: l.userId,
    date: l.date.toISOString(),
    minutes: l.minutes,
    subject: l.subject,
    textbookId: l.textbookId,
    textbook: l.textbook
      ? {
          id: l.textbook.id,
          masterId: l.textbook.masterId,
          name: l.textbook.name,
          totalAmount: l.textbook.totalAmount,
          rangeUnit: l.textbook.rangeUnit,
          targetDate: l.textbook.targetDate?.toISOString() ?? null,
          subject: l.textbook.subject,
        }
      : null,
    rangeStart: l.rangeStart,
    rangeEnd: l.rangeEnd,
    rangeUnit: l.rangeUnit,
    memo: l.memo,
    createdAt: l.createdAt.toISOString(),
    updatedAt: l.updatedAt.toISOString(),
  }));
  const initialTextbooks = textbooks.map((textbook) => ({
    id: textbook.id,
    masterId: textbook.masterId,
    name: textbook.name,
    totalAmount: textbook.totalAmount,
    rangeUnit: textbook.rangeUnit,
    targetDate: textbook.targetDate?.toISOString() ?? null,
    subject: textbook.subject,
  }));

  // ④ 志望校サマリー
  const firstChoice = decidedGoals.find((g) => g.isFirstChoice) ?? null;
  const otherCount = decidedGoals.filter((g) => !g.isFirstChoice).length;
  const candidateCount = goals.length - decidedGoals.length;
  return (
    <main className="w-full mx-auto max-w-3xl p-8">
      <h1 className="text-3xl font-bold mb-6">ダッシュボード</h1>

      <section className="mb-8">
        <h2 className="text-xl font-bold mb-3">受験カウントダウン</h2>
        <ExamCountdown goals={countdownGoals} />
      </section>

      <section className="mb-8">
        <h2 className="text-xl font-bold mb-3">今日の学習予定</h2>
        <TodayStudyPlans plans={todayPlans} weekCount={weekCount} />
      </section>

      {/* 学習の記録・科目別・実績記録（クライアントで集計してリアクティブに更新） */}
      <StudyRecordDashboard
        initialLogs={initialLogs}
        initialTextbooks={initialTextbooks}
        firstChoiceExamDate={firstChoice?.faculty.examDate.toISOString() ?? null}
      />

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
                  {candidateCount > 0 && ` ・ 検討中：${candidateCount} 校`}
                </p>
              </div>
              <Link
                href="/goals"
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
