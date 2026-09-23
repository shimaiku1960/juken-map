import type { FastifyInstance } from "fastify";
import {
  createTextbookSchema,
  updateTextbookProgressSchema,
} from "@/shared/validations/textbook";
import {
  createTextbook,
  createTextbookFromMaster,
  findOwnedTextbook,
  listTextbooks,
  updateTextbookProgress,
} from "@/api/services/textbook-service";
import { denyDemoWrite, requireSession } from "../context.ts";

type IdParams = { id: string };

export function registerTextbookRoutes(app: FastifyInstance) {
  app.get("/api/textbooks", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    return listTextbooks(session.user.id);
  });

  app.post("/api/textbooks", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = createTextbookSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const outcome =
      "masterId" in parsed.data
        ? await createTextbookFromMaster(session.user.id, parsed.data.masterId)
        : await createTextbook({
            name: parsed.data.name,
            userId: session.user.id,
            subject: parsed.data.subject,
            rangeUnit: parsed.data.rangeUnit,
          });

    switch (outcome.result) {
      case "master_not_found":
        return reply.code(404).send({ error: "参考書マスターが見つかりません" });
      case "master_without_metric":
        return reply.code(400).send({ error: "参考書の総量データが登録されていません" });
      case "duplicate":
        return reply.code(409).send({ error: "この参考書はすでに登録されています" });
      case "ok":
        return reply.code(201).send(outcome.value);
    }
  });

  app.patch<{ Params: IdParams }>("/api/textbooks/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const textbookId = Number(request.params.id);
    if (!Number.isInteger(textbookId)) {
      return reply.code(400).send({ error: "Invalid textbook id" });
    }

    const parsed = updateTextbookProgressSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const textbook = await findOwnedTextbook(textbookId, session.user.id);
    if (!textbook) {
      return reply.code(404).send({ error: "参考書が見つかりません" });
    }

    return updateTextbookProgress(textbookId, parsed.data);
  });
}
