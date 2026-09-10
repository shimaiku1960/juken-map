import type { FastifyInstance } from "fastify";
import { prisma } from "@/api/infra/prisma";
import { createStudyLogSchema } from "@/shared/validations/studyLog";
import { denyDemoWrite, requireSession } from "../context.ts";

type IdParams = { id: string };

function findOwnedTextbook(textbookId: number, userId: string) {
  return prisma.textbook.findFirst({ where: { id: textbookId, userId } });
}

function textbookValidationError(
  textbook: { rangeUnit: string | null; totalAmount: number | null },
  data: { rangeEnd?: number | null; rangeUnit?: string | null }
) {
  if (
    data.rangeEnd != null &&
    textbook.rangeUnit != null &&
    data.rangeUnit !== textbook.rangeUnit
  ) {
    return "範囲の単位を参考書の逆算設定に合わせてください";
  }
  if (
    data.rangeEnd != null &&
    textbook.totalAmount != null &&
    data.rangeEnd > textbook.totalAmount
  ) {
    return `終了位置は参考書の総量（${textbook.totalAmount}）以下にしてください`;
  }
  return null;
}

export function registerStudyLogItemRoutes(app: FastifyInstance) {
  app.patch<{ Params: IdParams }>("/api/study-logs/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const logId = Number(request.params.id);
    const log = await prisma.studyLog.findUnique({ where: { id: logId } });
    if (!log || log.userId !== session.user.id) {
      return reply.code(404).send({ error: "Not found" });
    }

    const parsed = createStudyLogSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const rangeChanged =
      parsed.data.rangeStart !== log.rangeStart ||
      parsed.data.rangeEnd !== log.rangeEnd ||
      parsed.data.rangeUnit !== log.rangeUnit;
    const textbookChanged =
      log.studyPlanId == null && parsed.data.textbookId !== log.textbookId;
    const effectiveTextbookId =
      log.studyPlanId != null ? log.textbookId : parsed.data.textbookId;

    // 時間・メモだけの修正では、後から変わった参考書設定を過去実績へ遡及しない。
    if (effectiveTextbookId != null && (rangeChanged || textbookChanged)) {
      const textbook = await findOwnedTextbook(effectiveTextbookId, session.user.id);
      if (!textbook) {
        return reply.code(400).send({ error: "不正な参考書です" });
      }
      const validationError = textbookValidationError(textbook, parsed.data);
      if (validationError) {
        return reply.code(400).send({ error: validationError });
      }
    }

    return prisma.studyLog.update({
      where: { id: logId },
      data: {
        // 予定から作成した実績は、予定との紐づきを壊す項目を固定する。
        date: log.studyPlanId != null ? log.date : new Date(parsed.data.date),
        minutes: parsed.data.minutes,
        subject: log.studyPlanId != null ? log.subject : parsed.data.subject ?? null,
        textbookId:
          log.studyPlanId != null ? log.textbookId : parsed.data.textbookId ?? null,
        rangeStart: parsed.data.rangeStart ?? null,
        rangeEnd: parsed.data.rangeEnd ?? null,
        rangeUnit: parsed.data.rangeUnit ?? null,
        memo: parsed.data.memo ?? null,
      },
    });
  });

  app.delete<{ Params: IdParams }>("/api/study-logs/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const id = Number(request.params.id);
    const log = await prisma.studyLog.findUnique({ where: { id } });
    if (!log || log.userId !== session.user.id) {
      return reply.code(404).send({ error: "Not found" });
    }

    await prisma.studyLog.delete({ where: { id } });
    return { message: "Deleted" };
  });
}
