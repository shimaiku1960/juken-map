import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { redirect } from "next/navigation";
import Link from "next/link";
import GoalList from "@/app/components/GoalList";
import ExamScheduleTimeline from "@/app/components/ExamScheduleTimeline";
import type { Goal } from "@/app/hooks/useGoals";
import { Card, CardContent } from "@/components/ui/card";

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

  return (
    <main className="w-full mx-auto max-w-3xl p-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-3xl font-bold">志望校</h1>
        <Link
          href="/explore"
          className="text-sm text-blue-500 hover:underline"
        >
          大学を探す →
        </Link>
      </div>
      <p className="mb-6 text-sm text-gray-500">
        第一志望と併願校を俯瞰して、受験する大学・学部を決めましょう。
      </p>

      <section className="mb-6">
        <h2 className="mb-2 text-sm font-bold text-gray-500">受験日程</h2>
        <ExamScheduleTimeline initialGoals={goals} />
      </section>

      <Card>
        <CardContent>
          <GoalList initialGoals={goals} faculties={faculties} />
        </CardContent>
      </Card>
    </main>
  );
};

export default GoalsPage;
