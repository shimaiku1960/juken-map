import { auth } from "@/lib/auth";
import { headers } from "next/headers";
import prisma from "@/lib/prisma";
import { redirect } from "next/navigation";
import Link from "next/link";
import StudySessionManager from "@/app/components/StudySessionManager";
import type { StudyPlan } from "@/app/hooks/useStudyPlans";
import { ymdLocal, todayYmd } from "@/lib/date";
import { DEMO_EMAIL } from "@/lib/demo";

const Home = async () => {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) redirect("/login");

  const todayStr = todayYmd();
  const [firstChoiceGoal, plans] = await Promise.all([
    prisma.finalGoal.findFirst({
      where: { userId: session.user.id, status: "decided", isFirstChoice: true },
      include: { faculty: { include: { university: true } } },
    }),
    prisma.studyPlan.findMany({
      where: { userId: session.user.id },
      orderBy: { date: "asc" },
      include: { textbook: true, studyLog: { select: { id: true } } },
    }),
  ]);

  const initialPlans: StudyPlan[] = plans.map((plan) => ({
    id: plan.id,
    userId: plan.userId,
    date: plan.date.toISOString(),
    content: plan.content,
    subject: plan.subject,
    done: plan.done,
    studyLogId: plan.studyLog?.id ?? null,
    textbookId: plan.textbookId,
    textbook: plan.textbook
      ? {
          id: plan.textbook.id,
          masterId: plan.textbook.masterId,
          name: plan.textbook.name,
          totalAmount: plan.textbook.totalAmount,
          rangeUnit: plan.textbook.rangeUnit,
          targetDate: plan.textbook.targetDate?.toISOString() ?? null,
          subject: plan.textbook.subject,
        }
      : null,
    rangeStart: plan.rangeStart,
    rangeEnd: plan.rangeEnd,
    rangeUnit: plan.rangeUnit,
    createdAt: plan.createdAt.toISOString(),
    updatedAt: plan.updatedAt.toISOString(),
  }));

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
          <span className="mx-1 text-lg font-bold text-foreground">あと {daysToExam} 日</span>
        </p>
      )}
      <h1 className="text-2xl font-bold sm:text-3xl">今日の学習を始めよう</h1>
      <div id="study-start" className="scroll-mt-24">
        <StudySessionManager
          initialPlans={initialPlans}
          userId={session.user.id}
          readOnly={session.user.email === DEMO_EMAIL}
          variant="hero"
        />
      </div>
      <Link href="/dashboard" className="mt-4 text-sm text-blue-600 hover:underline">
        今日の予定・記録を見る →
      </Link>
    </main>
  );
};

export default Home;
