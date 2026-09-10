import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

export function listStudyLogs(userId: string) {
  return measured("studyLog.list", () =>
    prisma.studyLog.findMany({
      where: { userId },
      orderBy: { date: "desc" },
      include: { textbook: true },
    })
  );
}

// ここから下は書き込み。HTTP は知らない。

/** 更新・削除の前に「自分のものか」を確かめる。 */
export function findOwnedStudyLog(id: number, userId: string) {
  return measured("studyLog.findOwned", () =>
    prisma.studyLog.findFirst({ where: { id, userId } })
  );
}

/**
 * 実績を1件記録する。「初回記録」の印付けと同じトランザクションで行う。
 *
 * updateMany の where に firstStudyLogAt: null を入れて DB 側で判定させている。
 * 先に読んでから書くと、同時アクセスで両方が「初回」になり得る。
 */
export function createStudyLog(input: {
  userId: string;
  date: string;
  minutes: number;
  subject?: string | null;
  textbookId?: number | null;
  rangeStart?: number | null;
  rangeEnd?: number | null;
  rangeUnit?: string | null;
  memo?: string | null;
}) {
  return measured("studyLog.create", () =>
    prisma.$transaction(async (tx) => {
      const activation = await tx.user.updateMany({
        where: { id: input.userId, firstStudyLogAt: null },
        data: { firstStudyLogAt: new Date() },
      });
      const created = await tx.studyLog.create({
        data: {
          date: new Date(input.date),
          minutes: input.minutes,
          subject: input.subject ?? null,
          textbookId: input.textbookId ?? null,
          rangeStart: input.rangeStart ?? null,
          rangeEnd: input.rangeEnd ?? null,
          rangeUnit: input.rangeUnit ?? null,
          memo: input.memo ?? null,
          userId: input.userId,
        },
      });
      return { created, isFirstStudyLog: activation.count === 1 };
    })
  );
}

/**
 * 実績を更新する。予定から作られた実績は、予定との紐づきを壊す項目
 * （日付・科目・参考書）を固定する。
 */
export function updateStudyLog(
  id: number,
  current: {
    studyPlanId: number | null;
    date: Date;
    subject: string | null;
    textbookId: number | null;
  },
  data: {
    date: string;
    minutes: number;
    subject?: string | null;
    textbookId?: number | null;
    rangeStart?: number | null;
    rangeEnd?: number | null;
    rangeUnit?: string | null;
    memo?: string | null;
  }
) {
  const fromPlan = current.studyPlanId != null;
  return measured("studyLog.update", () =>
    prisma.studyLog.update({
      where: { id },
      data: {
        date: fromPlan ? current.date : new Date(data.date),
        minutes: data.minutes,
        subject: fromPlan ? current.subject : data.subject ?? null,
        textbookId: fromPlan ? current.textbookId : data.textbookId ?? null,
        rangeStart: data.rangeStart ?? null,
        rangeEnd: data.rangeEnd ?? null,
        rangeUnit: data.rangeUnit ?? null,
        memo: data.memo ?? null,
      },
    })
  );
}

export function deleteStudyLog(id: number) {
  return measured("studyLog.delete", () =>
    prisma.studyLog.delete({ where: { id } })
  );
}
