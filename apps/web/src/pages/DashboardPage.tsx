import { Link } from "react-router";
import StudyRecordDashboard from "@/web/components/StudyRecordDashboard";
import { Card, CardContent } from "@/web/components/ui/card";
import { DEMO_EMAIL } from "@/shared/demo";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import SectionHeader from "@/web/components/layout/SectionHeader";
import { buttonVariants } from "@/web/components/ui/button";
import { cn } from "@/web/lib/utils";
import RegistrationCompletionTracker from "@/web/components/analytics/RegistrationCompletionTracker";
import { useGoals } from "@/web/hooks/useGoals";
import { useSession } from "@/web/lib/auth-client";

export default function DashboardPage() {
  const { data: session } = useSession();
  // Next.js 版はサーバーで listGoalsWithFaculty を呼んでいた。SPA では HTTP 経由になる。
  const { data: goals = [] } = useGoals();

  const decidedGoals = goals.filter((goal) => goal.status === "decided");
  const firstChoice = decidedGoals.find((goal) => goal.isFirstChoice) ?? null;
  const otherCount = decidedGoals.filter((goal) => !goal.isFirstChoice).length;
  const candidateCount = goals.length - decidedGoals.length;

  return (
    <PageShell>
      <RegistrationCompletionTracker />
      <PageHeader
        title="記録・予定"
        description="学習予定と実績をまとめて確認できます。"
      />

      <StudyRecordDashboard readOnly={session?.user.email === DEMO_EMAIL} />

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
                to="/goals"
                className={cn(
                  buttonVariants({ variant: "link", size: "lg" }),
                  "h-11"
                )}
              >
                志望校を設定 →
              </Link>
            </div>
          </CardContent>
        </Card>
      </section>
    </PageShell>
  );
}
