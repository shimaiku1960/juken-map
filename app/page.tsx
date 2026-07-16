import { auth } from "@/lib/auth";
import { headers } from "next/headers";
import prisma from "@/lib/prisma";
import { redirect } from "next/navigation";
import Link from "next/link";
import { type TodayPlan } from "@/app/components/TodayStudyPlans";
import StudySessionManager from "@/app/components/StudySessionManager";
import { ymdLocal, todayYmd } from "@/lib/date";
import { studyPlanLabel } from "@/lib/studyPlan";
import { DEMO_EMAIL } from "@/lib/demo";

const Home = async () => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    redirect("/login");
  }

  // トップページは「学習を始める」ことだけに集中できるよう、
  // 今日の予定と第一志望のカウントダウンに必要な最小限だけを取得する。
  const todayStr = todayYmd();
  const [firstChoiceGoal, plans, linkedLogs] = await Promise.all([
    prisma.finalGoal.findFirst({
      where: {
        userId: session.user.id,
        status: "decided",
        isFirstChoice: true,
      },
      include: { faculty: { include: { university: true } } },
    }),
    prisma.studyPlan.findMany({
      where: { userId: session.user.id },
      orderBy: { date: "asc" },
      include: { textbook: true },
    }),
    prisma.studyLog.findMany({
      where: { userId: session.user.id, studyPlanId: { not: null } },
      select: { studyPlanId: true, minutes: true },
    }),
  ]);

  const todayPlans: TodayPlan[] = plans
    .filter((p) => ymdLocal(p.date) === todayStr)
    .map((p) => {
      const linkedLog = linkedLogs.find((log) => log.studyPlanId === p.id);
      return {
        id: p.id,
        content: studyPlanLabel(p),
        done: p.done,
        subject: p.subject,
        textbookId: p.textbookId,
        textbookName: p.textbook?.name ?? null,
        rangeStart: p.rangeStart,
        rangeEnd: p.rangeEnd,
        rangeUnit: p.rangeUnit,
        recordedMinutes: linkedLog?.minutes ?? null,
      };
    });

  // 第一志望の受験日を一言だけ添えてモチベーションにする。
  const heroFirstChoice = firstChoiceGoal
    ? {
        name: `${firstChoiceGoal.faculty.university.name} ${firstChoiceGoal.faculty.name}`,
        examDate: firstChoiceGoal.faculty.examDate,
      }
    : null;
  const daysToExam = heroFirstChoice
    ? Math.ceil(
        (new Date(ymdLocal(heroFirstChoice.examDate)).getTime() -
          new Date(todayStr).getTime()) /
          86_400_000
      )
    : null;

  return (
    <main className="w-full mx-auto flex min-h-[calc(100dvh-4rem)] max-w-3xl flex-col items-center justify-center gap-6 p-8 text-center">
      {heroFirstChoice && daysToExam != null && daysToExam >= 0 && (
        <p className="text-sm text-muted-foreground">
          {heroFirstChoice.name} まで
          <span className="mx-1 text-lg font-bold text-foreground">
            あと {daysToExam} 日
          </span>
        </p>
      )}
      <p className="text-2xl font-bold sm:text-3xl">今日の学習を始めよう</p>
      <StudySessionManager
        plans={todayPlans}
        userId={session.user.id}
        readOnly={session.user.email === DEMO_EMAIL}
        variant="hero"
      />
      <Link
        href="/dashboard"
        className="mt-4 text-sm text-blue-600 hover:underline"
      >
        今日の予定・記録を見る →
      </Link>
    </main>
  );
};

export default Home;
