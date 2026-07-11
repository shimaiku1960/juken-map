import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { redirect } from "next/navigation";
import StudyPlanCalendar from "@/app/components/StudyPlanCalendar";
import type { StudyPlan } from "@/app/hooks/useStudyPlans";
import type { Goal } from "@/app/hooks/useGoals";

const SchedulePage = async () => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    redirect("/login");
  }

  const plansRaw = await prisma.studyPlan.findMany({
    where: { userId: session.user.id },
    orderBy: { date: "asc" },
    include: { textbook: true },
  });

  // Date を JSON と同じ ISO 文字列に揃えてクライアントへ渡す（useStudyPlans の型に合わせる）
  const initialPlans: StudyPlan[] = plansRaw.map((p) => ({
    id: p.id,
    userId: p.userId,
    date: p.date.toISOString(),
    content: p.content,
    subject: p.subject,
    done: p.done,
    textbookId: p.textbookId,
    textbook: p.textbook
      ? {
          id: p.textbook.id,
          masterId: p.textbook.masterId,
          name: p.textbook.name,
          totalAmount: p.textbook.totalAmount,
          rangeUnit: p.textbook.rangeUnit,
          targetDate: p.textbook.targetDate?.toISOString() ?? null,
        }
      : null,
    rangeStart: p.rangeStart,
    rangeEnd: p.rangeEnd,
    rangeUnit: p.rangeUnit,
    createdAt: p.createdAt.toISOString(),
    updatedAt: p.updatedAt.toISOString(),
  }));

  // 志望校（受験日程）を取得。カレンダーに読み取り専用で重ね、クリックで
  // ユーザー資産（第一志望/メモ/削除）を編集できるようにする
  const goalsRaw = await prisma.finalGoal.findMany({
    where: { userId: session.user.id },
    include: { faculty: { include: { university: true, tags: true } } },
    orderBy: { createdAt: "asc" },
  });

  const initialGoals: Goal[] = goalsRaw.map((g) => ({
    id: g.id,
    createdAt: g.createdAt,
    userId: g.userId,
    isFirstChoice: g.isFirstChoice,
    note: g.note,
    faculty: {
      id: g.faculty.id,
      name: g.faculty.name,
      examDate: g.faculty.examDate,
      university: { name: g.faculty.university.name },
      tags: g.faculty.tags.map((t) => ({ name: t.name })),
    },
  }));

  return (
    <main className="w-full mx-auto max-w-3xl p-8">
      <h1 className="text-3xl font-bold mb-6">学習予定</h1>
      <StudyPlanCalendar initialPlans={initialPlans} initialGoals={initialGoals} />
    </main>
  );
};

export default SchedulePage;
