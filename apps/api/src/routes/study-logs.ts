import type { FastifyInstance } from "fastify";
import { createStudyLogSchema } from "@/shared/validations/studyLog";
import {
  createStudyLog,
  listStudyLogs,
} from "@/api/services/study-log-service";
import { findOwnedTextbook } from "@/api/services/textbook-service";
import { textbookRangeError } from "@/api/domain/textbookRange";
import { denyDemoWrite, requireSession } from "../context.ts";

export function registerStudyLogRoutes(app: FastifyInstance) {
  app.get("/api/study-logs", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    return listStudyLogs(session.user.id);
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
      const textbook = await findOwnedTextbook(
        parsed.data.textbookId,
        session.user.id
      );
      if (!textbook) {
        return reply.code(400).send({ error: "不正な参考書です" });
      }

      const rangeError = textbookRangeError(textbook, parsed.data);
      if (rangeError) {
        return reply.code(400).send({ error: rangeError });
      }
    }

    const { created, isFirstStudyLog } = await createStudyLog({
      userId: session.user.id,
      ...parsed.data,
    });

    return reply.code(201).send({ ...created, isFirstStudyLog });
  });
}
