import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { redirect } from "next/navigation";
import Link from "next/link";
import GoalList from "@/app/components/GoalList";
import ExamScheduleTimeline from "@/app/components/ExamScheduleTimeline";
import type { Goal } from "@/app/hooks/useGoals";
import { Card, CardContent } from "@/components/ui/card";
import { SUBJECTS, subjectColor, subjectLabel } from "@/lib/subjects";
import { DEMO_EMAIL } from "@/lib/demo";
import { Target } from "lucide-react";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";
import SectionHeader from "@/app/components/layout/SectionHeader";
import EmptyState from "@/app/components/feedback/EmptyState";
import { Button, buttonVariants } from "@/components/ui/button";

// 志望校ページ＝「受験戦略を俯瞰し、受験校を決める」場所。
// 以前はプロフィール下部に埋もれていた GoalList をここへ独立させた。
const GoalsPage = async () => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    redirect("/login");
  }

  const goalsRaw = await prisma.finalGoal.findMany({
    where: { userId: session.user.id },
    include: {
      faculty: {
        include: { university: true, tags: true },
      },
    },
    orderBy: { createdAt: "asc" },
  });

  // Prisma の status(string) を Goal 型（"candidate" | "decided"）に揃える
  const goals: Goal[] = goalsRaw.map((g) => ({
    ...g,
    status: g.status as Goal["status"],
  }));

  const faculties = await prisma.faculty.findMany({
    include: { university: true, tags: true },
    orderBy: { id: "asc" },
  });

  // 対策科目＝手持ち参考書を科目別に集計。志望校（受験日）→科目→参考書→
  // 今日のノルマ、という計画〜実行の因果をこのページからも辿れるようにする。
  const textbooks = await prisma.textbook.findMany({
    where: { userId: session.user.id },
    select: { subject: true },
  });
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

  return (
    <PageShell>
      <PageHeader
        title="志望校"
        description="第一志望と併願校を俯瞰して、受験する大学・学部を決めましょう。"
        action={goals.length > 0 ? <Button asChild><Link href="/explore">大学を探す</Link></Button> : undefined}
      />

      {goals.length === 0 ? (
        <EmptyState
          icon={Target}
          title="志望校を登録して受験計画を始めよう"
          description="大学・学部を登録すると、試験日や併願校、必要な対策をまとめて整理できます。"
          action={<Button asChild size="lg" className="h-11"><Link href="/explore">大学を探す</Link></Button>}
        />
      ) : (
      <>

      <section className="mb-6">
        <SectionHeader title="受験日程" />
        <ExamScheduleTimeline initialGoals={goals} />
      </section>

      <section className="mb-6">
        <SectionHeader title="対策の科目" />
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
                  <Link href="/" className={buttonVariants({ variant: "link", size: "sm" })}>
                    「今日やること」
                  </Link>{" "}
                  で確認できます。
                </p>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">
                参考書に科目を設定すると、対策科目がここに表示されます。参考書は{" "}
                <Link href="/dashboard" className={buttonVariants({ variant: "link", size: "sm" })}>
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
          <GoalList
            initialGoals={goals}
            faculties={faculties}
            readOnly={session.user.email === DEMO_EMAIL}
          />
        </CardContent>
      </Card>
      </>
      )}
    </PageShell>
  );
};

export default GoalsPage;
