import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

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

// ここから下は書き込み。HTTP のことは知らないので、
// 「見つからない」は null、「一意制約違反」は Prisma の例外のまま呼び出し元へ返す。
// それを 404 や 409 にするかは routes の判断。

/** 更新・削除の前に「自分のものか」を確かめる。他人のものと存在しないものを区別しない。 */
export function findOwnedGoal(id: number, userId: string) {
  return measured("goal.findOwned", () =>
    prisma.finalGoal.findFirst({ where: { id, userId } })
  );
}

/** 志望校を登録する。同じ学部の重複は DB の一意制約が弾く（P2002）。 */
export function createGoal(input: {
  userId: string;
  facultyId: number;
  status?: string;
}) {
  return measured("goal.create", () =>
    prisma.finalGoal.create({
      data: {
        facultyId: input.facultyId,
        userId: input.userId,
        status: input.status ?? "decided",
      },
      include: { faculty: { include: { university: true } } },
    })
  );
}

/**
 * 志望校を更新する。facultyId は任意で、未指定なら中身を変えずに更新だけ走る
 * （updatedAt が動く）。呼び出し前の挙動をそのまま保つため、この形にしてある。
 */
export function updateGoal(id: number, data: { facultyId?: number }) {
  return measured("goal.update", () =>
    prisma.finalGoal.update({
      where: { id },
      data: { ...(data.facultyId && { facultyId: data.facultyId }) },
    })
  );
}

/**
 * 第一志望・メモ・ステータスのうち、送られてきたものだけを更新する。
 *
 * 第一志望は1ユーザー1校までなので、付け替えは「全部外す→1件立てる」を
 * ひとつのトランザクションで行う。分けて実行すると、途中で失敗したときに
 * 第一志望が0校の状態が残る。
 */
export function applyGoalPatch(
  userId: string,
  id: number,
  patch: { isFirstChoice?: boolean; note?: string | null; status?: string }
) {
  return measured("goal.applyPatch", async () => {
    if (patch.isFirstChoice !== undefined) {
      if (patch.isFirstChoice) {
        await prisma.$transaction([
          prisma.finalGoal.updateMany({
            where: { userId },
            data: { isFirstChoice: false },
          }),
          prisma.finalGoal.update({ where: { id }, data: { isFirstChoice: true } }),
        ]);
      } else {
        await prisma.finalGoal.update({ where: { id }, data: { isFirstChoice: false } });
      }
    }

    if (patch.note !== undefined) {
      await prisma.finalGoal.update({ where: { id }, data: { note: patch.note } });
    }

    if (patch.status !== undefined) {
      await prisma.finalGoal.update({ where: { id }, data: { status: patch.status } });
    }
  });
}

/** 志望校を削除する。 */
export function deleteGoal(id: number) {
  return measured("goal.delete", () =>
    prisma.finalGoal.delete({ where: { id } })
  );
}
