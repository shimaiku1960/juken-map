import type { FastifyInstance } from "fastify";
import { prisma } from "@/api/infra/prisma";
import { createStudyLogSchema } from "@/shared/validations/studyLog";
import { toStudyLogDTO } from "@/api/dto/study-mapper";
import { listStudyLogs } from "@/api/services/study-log-service";
import { denyDemoWrite, requireSession } from "../context.ts";

export function registerStudyLogRoutes(app: FastifyInstance) {
  app.get("/api/study-logs", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    const logs = await listStudyLogs(session.user.id);
    return logs.map(toStudyLogDTO);
  });

  app.post("/api/study-logs", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = createStudyLogSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    // 参考書を指定する場合は、所有権と逆算設定との整合性を検証する。
    if (parsed.data.textbookId != null) {
      const textbook = await prisma.textbook.findFirst({
        where: { id: parsed.data.textbookId, userId: session.user.id },
      });
      if (!textbook) {
        return reply.code(400).send({ error: "不正な参考書です" });
      }

      if (
        parsed.data.rangeEnd != null &&
        textbook.rangeUnit != null &&
        parsed.data.rangeUnit !== textbook.rangeUnit
      ) {
        return reply
          .code(400)
          .send({ error: "範囲の単位を参考書の逆算設定に合わせてください" });
      }

      if (
        parsed.data.rangeEnd != null &&
        textbook.totalAmount != null &&
        parsed.data.rangeEnd > textbook.totalAmount
      ) {
        return reply.code(400).send({
          error: `終了位置は参考書の総量（${textbook.totalAmount}）以下にしてください`,
        });
      }
    }

    const { created, isFirstStudyLog } = await prisma.$transaction(async (tx) => {
      const activation = await tx.user.updateMany({
        where: { id: session.user.id, firstStudyLogAt: null },
        data: { firstStudyLogAt: new Date() },
      });
      const created = await tx.studyLog.create({
        data: {
          date: new Date(parsed.data.date),
          minutes: parsed.data.minutes,
          subject: parsed.data.subject ?? null,
          textbookId: parsed.data.textbookId ?? null,
          rangeStart: parsed.data.rangeStart ?? null,
          rangeEnd: parsed.data.rangeEnd ?? null,
          rangeUnit: parsed.data.rangeUnit ?? null,
          memo: parsed.data.memo ?? null,
          userId: session.user.id,
        },
      });
      return { created, isFirstStudyLog: activation.count === 1 };
    });

    return reply.code(201).send({ ...created, isFirstStudyLog });
  });
}
