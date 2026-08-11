import type { Metadata } from "next";
import { auth } from "@/lib/auth";
import { headers } from "next/headers";
import prisma from "@/lib/prisma";
import { NOINDEX } from "@/lib/site";
import { redirect } from "next/navigation";
import Link from "next/link";
import ExamCountdown, { type CountdownGoal } from "@/app/components/ExamCountdown";
import StudyRecordDashboard from "@/app/components/StudyRecordDashboard";
import type { StudyLog } from "@/app/hooks/useStudyLogs";
import type { StudyPlan } from "@/app/hooks/useStudyPlans";
import { Card, CardContent } from "@/components/ui/card";
import { DEMO_EMAIL } from "@/lib/demo";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";
import SectionHeader from "@/app/components/layout/SectionHeader";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import SupportSubscriptionCard from "@/app/components/SupportSubscriptionCard";
import { hasSupportLineAccess } from "@/lib/support-line";

// ログイン必須のページなので検索結果には載せない。
export const metadata: Metadata = { robots: NOINDEX };

const DashboardPage = async () => {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) redirect("/login");

  const [goals, plans, logsRaw, supportSubscription, supportLineConnection] = await Promise.all([
    prisma.finalGoal.findMany({
      where: { userId: session.user.id },
      include: { faculty: { include: { university: true } } },
      orderBy: { createdAt: "asc" },
    }),
    prisma.studyPlan.findMany({
      where: { userId: session.user.id },
      orderBy: { date: "asc" },
      include: { textbook: true, studyLog: { select: { id: true } } },
    }),
    prisma.studyLog.findMany({
      where: { userId: session.user.id },
      orderBy: { date: "desc" },
      include: { textbook: true },
    }),
    prisma.supportSubscription.findUnique({
      where: { userId: session.user.id },
      select: {
        status: true,
        trialEndsAt: true,
        currentPeriodEndsAt: true,
        cancelAtPeriodEnd: true,
        cancelAt: true,
      },
    }),
    prisma.supportLineConnection.findUnique({
      where: { userId: session.user.id },
      select: { linkedAt: true },
    }),
  ]);

  const decidedGoals = goals.filter((goal) => goal.status === "decided");
  const countdownGoals: CountdownGoal[] = decidedGoals.map((goal) => ({
    id: goal.id,
    universityName: goal.faculty.university.name,
    facultyName: goal.faculty.name,
    examDate: goal.faculty.examDate,
    isFirstChoice: goal.isFirstChoice,
  }));

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

  const initialLogs: StudyLog[] = logsRaw.map((log) => ({
    id: log.id,
    userId: log.userId,
    date: log.date.toISOString(),
    minutes: log.minutes,
    subject: log.subject,
    textbookId: log.textbookId,
    textbook: log.textbook
      ? {
          id: log.textbook.id,
          masterId: log.textbook.masterId,
          name: log.textbook.name,
          totalAmount: log.textbook.totalAmount,
          rangeUnit: log.textbook.rangeUnit,
          targetDate: log.textbook.targetDate?.toISOString() ?? null,
          subject: log.textbook.subject,
        }
      : null,
    rangeStart: log.rangeStart,
    rangeEnd: log.rangeEnd,
    rangeUnit: log.rangeUnit,
    memo: log.memo,
    studyPlanId: log.studyPlanId,
    createdAt: log.createdAt.toISOString(),
    updatedAt: log.updatedAt.toISOString(),
  }));

  const firstChoice = decidedGoals.find((goal) => goal.isFirstChoice) ?? null;
  const otherCount = decidedGoals.filter((goal) => !goal.isFirstChoice).length;
  const candidateCount = goals.length - decidedGoals.length;

  return (
    <PageShell>
      <PageHeader title="記録・予定" description="学習予定と実績をまとめて確認できます。" />

      <section className="mb-8">
        <SectionHeader title="受験カウントダウン" />
        <ExamCountdown goals={countdownGoals} />
      </section>

      <StudyRecordDashboard
        initialLogs={initialLogs}
        initialPlans={initialPlans}
        readOnly={session.user.email === DEMO_EMAIL}
      />

      {supportSubscription ? (
        <section className="mb-8">
          <SectionHeader title="英語質問サポート" />
          <SupportSubscriptionCard
            status={supportSubscription.status}
            trialEndsAt={supportSubscription.trialEndsAt?.toISOString() ?? null}
            currentPeriodEndsAt={
              supportSubscription.currentPeriodEndsAt?.toISOString() ?? null
            }
            cancelAtPeriodEnd={supportSubscription.cancelAtPeriodEnd}
            cancelAt={supportSubscription.cancelAt?.toISOString() ?? null}
            lineAccessEnabled={hasSupportLineAccess(supportSubscription.status)}
            lineLinked={Boolean(supportLineConnection?.linkedAt)}
          />
        </section>
      ) : null}

      <section className="mb-8">
        <SectionHeader title="志望校" />
        <Card>
          <CardContent className="py-5">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">第一志望</p>
                <p className="font-medium">
                  {firstChoice
                    ? `${firstChoice.faculty.university.name} ${firstChoice.faculty.name}`
                    : "未設定"}
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  併願：{otherCount} 校
                  {candidateCount > 0 && ` ・ 検討中：${candidateCount} 校`}
                </p>
              </div>
              <Link
                href="/goals"
                className={cn(buttonVariants({ variant: "link", size: "lg" }), "h-11")}
              >
                志望校を設定 →
              </Link>
            </div>
          </CardContent>
        </Card>
      </section>

      <p className="text-sm text-muted-foreground">
        ※ 表示している受験日は暫定です。正式な日程は各大学の募集要項で必ずご確認ください。
      </p>
    </PageShell>
  );
};

export default DashboardPage;
