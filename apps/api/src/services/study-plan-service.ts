import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

export function listStudyPlans(userId: string) {
  return measured("studyPlan.list", () =>
    prisma.studyPlan.findMany({
      where: { userId },
      orderBy: { date: "asc" },
      include: { textbook: true, studyLog: { select: { id: true } } },
    })
  );
}

// ここから下は書き込み。HTTP は知らない。

/** 更新・削除の前に「自分のものか」を確かめる。 */
export function findOwnedStudyPlan(id: number, userId: string) {
  return measured("studyPlan.findOwned", () =>
    prisma.studyPlan.findFirst({ where: { id, userId } })
  );
}

/** 完了処理用。参考書と、既に紐づく実績まで一度に引く。 */
export function findOwnedStudyPlanForComplete(id: number, userId: string) {
  return measured("studyPlan.findOwnedForComplete", () =>
    prisma.studyPlan.findFirst({
      where: { id, userId },
      include: { textbook: true, studyLog: true },
    })
  );
}

/** 1つの日付に複数の予定をまとめて作る。 */
export function createStudyPlans(input: {
  userId: string;
  date: Date;
  items: {
    textbookId?: number | null;
    rangeStart?: number | null;
    rangeEnd?: number | null;
    rangeUnit?: string | null;
    content?: string | null;
    subject?: string | null;
  }[];
}) {
  return measured("studyPlan.createMany", () =>
    prisma.studyPlan.createMany({
      data: input.items.map((item) => ({
        date: input.date,
        textbookId: item.textbookId ?? null,
        rangeStart: item.rangeStart ?? null,
        rangeEnd: item.rangeEnd ?? null,
        rangeUnit: item.rangeUnit ?? null,
        content: item.content ?? null,
        subject: item.subject ?? null,
        userId: input.userId,
      })),
    })
  );
}

/** この予定に紐づく実績の件数。完了を取り消してよいかの判断に使う。 */
export function countLinkedStudyLogs(planId: number) {
  return measured("studyPlan.countLinkedLogs", () =>
    prisma.studyLog.count({ where: { studyPlanId: planId } })
  );
}

/** 送られてきた項目だけを更新する。 */
export function updateStudyPlan(
  id: number,
  data: {
    date?: string;
    content?: string | null;
    subject?: string | null;
    textbookId?: number | null;
    rangeStart?: number | null;
    rangeEnd?: number | null;
    rangeUnit?: string | null;
    done?: boolean;
  }
) {
  return measured("studyPlan.update", () =>
    prisma.studyPlan.update({
      where: { id },
      data: {
        ...(data.date && { date: new Date(data.date) }),
        ...(data.content !== undefined && { content: data.content }),
        ...(data.subject !== undefined && { subject: data.subject }),
        ...(data.textbookId !== undefined && { textbookId: data.textbookId }),
        ...(data.rangeStart !== undefined && { rangeStart: data.rangeStart }),
        ...(data.rangeEnd !== undefined && { rangeEnd: data.rangeEnd }),
        ...(data.rangeUnit !== undefined && { rangeUnit: data.rangeUnit }),
        ...(data.done !== undefined && { done: data.done }),
      },
    })
  );
}

export function deleteStudyPlan(id: number) {
  return measured("studyPlan.delete", () =>
    prisma.studyPlan.delete({ where: { id } })
  );
}

/**
 * 予定を完了にし、同時に実績を1件作る。
 *
 * 実績の作成と予定の完了は必ず一緒に成立させる。片方だけ成功すると
 * 「完了なのに実績が無い」または「実績はあるのに未完了」という状態が残る。
 *
 * 「初回記録」の印は updateMany の where に firstStudyLogAt: null を入れて
 * DB 側で判定させる。先に読んでから書くと、同時アクセスで両方が初回になり得る。
 */
export function completeStudyPlan(input: {
  userId: string;
  plan: { id: number; date: Date; subject: string | null; textbookId: number | null };
  minutes: number;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  memo: string | null;
}) {
  return measured("studyPlan.complete", () =>
    prisma.$transaction(async (tx) => {
      const activation = await tx.user.updateMany({
        where: { id: input.userId, firstStudyLogAt: null },
        data: { firstStudyLogAt: new Date() },
      });
      const log = await tx.studyLog.create({
        data: {
          userId: input.userId,
          studyPlanId: input.plan.id,
          date: input.plan.date,
          minutes: input.minutes,
          subject: input.plan.subject,
          textbookId: input.plan.textbookId,
          rangeStart: input.rangeStart,
          rangeEnd: input.rangeEnd,
          rangeUnit: input.rangeUnit,
          memo: input.memo,
        },
        include: { textbook: true },
      });
      const updatedPlan = await tx.studyPlan.update({
        where: { id: input.plan.id },
        data: { done: true },
      });
      return { log, updatedPlan, isFirstStudyLog: activation.count === 1 };
    })
  );
}
