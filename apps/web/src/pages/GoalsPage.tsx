import { Link } from "react-router";
import { Target } from "lucide-react";
import GoalList from "@/web/components/GoalList";
import ExamScheduleTimeline from "@/web/components/ExamScheduleTimeline";
import { Card, CardContent } from "@/web/components/ui/card";
import { SUBJECTS, subjectColor, subjectLabel } from "@/shared/subjects";
import { DEMO_EMAIL } from "@/shared/demo";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import SectionHeader from "@/web/components/layout/SectionHeader";
import EmptyState from "@/web/components/feedback/EmptyState";
import { Button, buttonVariants } from "@/web/components/ui/button";
import { useGoals } from "@/web/hooks/useGoals";
import { useTextbooks } from "@/web/hooks/useTextbooks";
import { useSession } from "@/web/lib/auth-client";

export default function GoalsPage() {
  const { data: session } = useSession();
  const { data: goals = [] } = useGoals();
  // Next.js 版は listTextbookSubjects でサーバー集計していた。
  // SPA では既存の /api/textbooks を使い、集計はここで行う。
  const { data: textbooks = [] } = useTextbooks();

  const subjectCounts = new Map<string, number>();
  for (const textbook of textbooks) {
    if (textbook.subject) {
      subjectCounts.set(
        textbook.subject,
        (subjectCounts.get(textbook.subject) ?? 0) + 1
      );
    }
  }
  const subjectChips = SUBJECTS.filter((s) => subjectCounts.has(s.value)).map(
    (s) => ({ value: s.value, count: subjectCounts.get(s.value) ?? 0 })
  );

  const readOnly = session?.user.email === DEMO_EMAIL;

  return (
    <PageShell>
      <PageHeader
        title="志望校"
        description="第一志望と併願校を並べて見ながら、受験する大学・学部を決めましょう。"
        action={
          goals.length > 0 ? (
            <Button asChild size="lg" className="h-11">
              <Link to="/explore">大学を探す</Link>
            </Button>
          ) : undefined
        }
      />

      {goals.length === 0 ? (
        <EmptyState
          icon={Target}
          title="志望校を登録して、受験の計画を立てよう"
          description="大学・学部を登録すると、試験日や併願校、必要な対策をまとめて整理できます。"
          action={
            <Button asChild size="lg" className="h-11">
              <Link to="/explore">大学を探す</Link>
            </Button>
          }
        />
      ) : (
        <>
          <section className="mb-6">
            <SectionHeader title="受験日程" />
            <ExamScheduleTimeline />
          </section>

          <section className="mb-6">
            <SectionHeader title="対策科目" />
            <Card>
              <CardContent className="py-5">
                {subjectChips.length > 0 ? (
                  <>
                    <div className="flex flex-wrap gap-2">
                      {subjectChips.map((chip) => (
                        <span
                          key={chip.value}
                          className="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm"
                          style={{
                            backgroundColor: `${subjectColor(chip.value)}1a`,
                            color: subjectColor(chip.value),
                          }}
                        >
                          {subjectLabel(chip.value)}
                          <span className="text-xs opacity-70">
                            参考書{chip.count}
                          </span>
                        </span>
                      ))}
                    </div>
                    <p className="mt-3 text-sm text-muted-foreground">
                      受験日から逆算した今日のノルマは{" "}
                      <Link
                        to="/"
                        className={buttonVariants({ variant: "link", size: "sm" })}
                      >
                        「今日やること」
                      </Link>{" "}
                      で確認できます。
                    </p>
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    参考書に科目を設定すると、対策科目がここに表示されます。参考書は{" "}
                    <Link
                      to="/dashboard"
                      className={buttonVariants({ variant: "link", size: "sm" })}
                    >
                      ダッシュボード
                    </Link>{" "}
                    の実績記録から追加できます。
                  </p>
                )}
              </CardContent>
            </Card>
          </section>

          <Card>
            <CardContent>
              <GoalList readOnly={readOnly} />
            </CardContent>
          </Card>
        </>
      )}
    </PageShell>
  );
}
