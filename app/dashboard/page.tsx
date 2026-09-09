import type { Metadata } from "next";
import { getCurrentSession } from "@/backend/infra/auth-session";
import { toStudyLogDTO, toStudyPlanDTO } from "@/backend/dto/study-mapper";
import { listGoalsWithFaculty } from "@/backend/services/goal-service";
import { listStudyLogs } from "@/backend/services/study-log-service";
import { listStudyPlans } from "@/backend/services/study-plan-service";
import { NOINDEX } from "@/shared/site";
import { redirect } from "next/navigation";
import Link from "next/link";
import StudyRecordDashboard from "@/frontend/components/StudyRecordDashboard";
import type { StudyLog } from "@/frontend/hooks/useStudyLogs";
import type { StudyPlan } from "@/frontend/hooks/useStudyPlans";
import { Card, CardContent } from "@/frontend/components/ui/card";
import { DEMO_EMAIL } from "@/shared/demo";
import PageShell from "@/frontend/components/layout/PageShell";
import PageHeader from "@/frontend/components/layout/PageHeader";
import SectionHeader from "@/frontend/components/layout/SectionHeader";
import { buttonVariants } from "@/frontend/components/ui/button";
import { cn } from "@/frontend/lib/utils";
import RegistrationCompletionTracker from "@/frontend/components/analytics/RegistrationCompletionTracker";

// ログイン必須のページなので検索結果には載せない。
export const metadata: Metadata = { robots: NOINDEX };

const DashboardPage = async () => {
  const session = await getCurrentSession();
  if (!session) redirect("/login");

  const [goals, plans, logsRaw] = await Promise.all([
    listGoalsWithFaculty(session.user.id),
    listStudyPlans(session.user.id),
    listStudyLogs(session.user.id),
  ]);

  const decidedGoals = goals.filter((goal) => goal.status === "decided");

  const initialPlans: StudyPlan[] = plans.map(toStudyPlanDTO);

  const initialLogs: StudyLog[] = logsRaw.map(toStudyLogDTO);

  const firstChoice = decidedGoals.find((goal) => goal.isFirstChoice) ?? null;
  const otherCount = decidedGoals.filter((goal) => !goal.isFirstChoice).length;
  const candidateCount = goals.length - decidedGoals.length;

  return (
    <PageShell>
      <RegistrationCompletionTracker />
      <PageHeader title="記録・予定" description="学習予定と実績をまとめて確認できます。" />

      <StudyRecordDashboard
        initialLogs={initialLogs}
        initialPlans={initialPlans}
        readOnly={session.user.email === DEMO_EMAIL}
      />

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
    </PageShell>
  );
};

export default DashboardPage;
