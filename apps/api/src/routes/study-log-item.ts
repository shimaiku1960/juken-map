import type { FastifyInstance } from "fastify";
import { createStudyLogSchema } from "@/shared/validations/studyLog";
import { textbookRangeError } from "@/api/domain/textbookRange";
import {
  deleteStudyLog,
  findOwnedStudyLog,
  updateStudyLog,
} from "@/api/services/study-log-service";
import { findOwnedTextbook } from "@/api/services/textbook-service";
import { denyDemoWrite, requireSession } from "../context.ts";

type IdParams = { id: string };

export function registerStudyLogItemRoutes(app: FastifyInstance) {
  app.patch<{ Params: IdParams }>("/api/study-logs/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const logId = Number(request.params.id);
    const log = await findOwnedStudyLog(logId, session.user.id);
    if (!log) {
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
      const rangeError = textbookRangeError(textbook, parsed.data);
      if (rangeError) {
        return reply.code(400).send({ error: rangeError });
      }
    }

    return updateStudyLog(logId, log, parsed.data);
  });

  app.delete<{ Params: IdParams }>("/api/study-logs/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const id = Number(request.params.id);
    const log = await findOwnedStudyLog(id, session.user.id);
    if (!log) {
      return reply.code(404).send({ error: "Not found" });
    }

    await deleteStudyLog(id);
    return { message: "Deleted" };
  });
}
