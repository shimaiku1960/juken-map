import prisma from "@/lib/infra/prisma";
import { measured } from "@/lib/observability/measured";

export function listGoals(userId: string) {
  return measured("goal.list", () =>
    prisma.finalGoal.findMany({
      where: { userId },
      orderBy: { createdAt: "asc" },
      include: {
        faculty: {
          include: { university: true, tags: true },
        },
      },
    })
  );
}

// ダッシュボード用。tags は画面で使わないため引かない（listGoals より SQL が1本少ない）。
// tags まで要るのは志望校ページだけなので、そちらは listGoals を使う。
export function listGoalsWithFaculty(userId: string) {
  return measured("goal.listWithFaculty", () =>
    prisma.finalGoal.findMany({
      where: { userId },
      orderBy: { createdAt: "asc" },
      include: { faculty: { include: { university: true } } },
    })
  );
}

// トップの「第一志望」表示専用。listGoals と違い tags を引かないので、
// 表示に必要な範囲だけで済む。
export function findFirstChoiceGoal(userId: string) {
  return measured("goal.findFirstChoice", () =>
    prisma.finalGoal.findFirst({
      where: { userId, status: "decided", isFirstChoice: true },
      include: { faculty: { include: { university: true } } },
    })
  );
}

// 志望校として登録済みの facultyId 一覧。大学詳細で「登録済み」を出し分けるのに使う。
export function listGoalFacultyIds(userId: string) {
  return measured("goal.listFacultyIds", () =>
    prisma.finalGoal.findMany({
      where: { userId },
      select: { facultyId: true },
    })
  );
}
