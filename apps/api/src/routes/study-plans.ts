import type { FastifyInstance } from "fastify";
import { isDuplicateEntry } from "@/api/infra/db";
import {
  createStudyPlansSchema,
  updateStudyPlanSchema,
} from "@/shared/validations/studyPlan";
import { completeStudyPlanSchema } from "@/shared/validations/studyLog";
import {
  completeStudyPlan,
  countLinkedStudyLogs,
  createStudyPlans,
  deleteStudyPlan,
  findOwnedStudyPlan,
  findOwnedStudyPlanForComplete,
  listStudyPlans,
  updateStudyPlan,
} from "@/api/services/study-plan-service";
import { countOwnedTextbooks } from "@/api/services/textbook-service";
import { denyDemoWrite, requireSession } from "../context.ts";

type IdParams = { id: string };

export function registerStudyPlanRoutes(app: FastifyInstance) {
  app.get("/api/study-plans", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    return listStudyPlans(session.user.id);
  });

  app.post("/api/study-plans", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = createStudyPlansSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    // 参考書は他人のIDを混ぜられないよう、自分の所有分だけを許可する
    const textbookIds = [
      ...new Set(
        parsed.data.items
          .map((item) => item.textbookId)
          .filter((id): id is number => id != null)
      ),
    ];

    if (textbookIds.length > 0) {
      const owned = await countOwnedTextbooks(textbookIds, session.user.id);
      if (owned !== textbookIds.length) {
        return reply.code(400).send({ error: "不正な参考書が含まれています" });
      }
    }

    const result = await createStudyPlans({
      userId: session.user.id,
      date: new Date(parsed.data.date),
      items: parsed.data.items,
    });

    return reply.code(201).send({ count: result.count });
  });

  app.patch<{ Params: IdParams }>("/api/study-plans/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = updateStudyPlanSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const id = Number(request.params.id);
    const plan = await findOwnedStudyPlan(id, session.user.id);
    if (!plan) {
      return reply.code(404).send({ error: "Not found" });
    }

    // 実績を記録済みの予定を未完了へ戻すと、実績だけが宙に浮く
    if (parsed.data.done === false) {
      const linkedLog = await countLinkedStudyLogs(plan.id);
      if (linkedLog > 0) {
        return reply
          .code(409)
          .send({ error: "実績を記録済みの予定は未完了に戻せません" });
      }
    }

    // 参考書を指定する場合は、自分の所有分だけを許可する
    if (parsed.data.textbookId != null) {
      const owned = await countOwnedTextbooks(
        [parsed.data.textbookId],
        session.user.id
      );
      if (owned === 0) {
        return reply.code(400).send({ error: "不正な参考書です" });
      }
    }

    return updateStudyPlan(id, parsed.data);
  });

  app.delete<{ Params: IdParams }>("/api/study-plans/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const id = Number(request.params.id);
    const plan = await findOwnedStudyPlan(id, session.user.id);
    if (!plan) {
      return reply.code(404).send({ error: "Not found" });
    }

    await deleteStudyPlan(id);
    return { message: "Deleted" };
  });

  app.post<{ Params: IdParams }>(
    "/api/study-plans/:id/complete",
    async (request, reply) => {
      const session = await requireSession(request, reply);
      if (!session) return;
      if (denyDemoWrite(session, reply)) return;

      const planId = Number(request.params.id);
      if (!Number.isInteger(planId) || planId <= 0) {
        return reply.code(404).send({ error: "Not found" });
      }

      const parsed = completeStudyPlanSchema.safeParse(request.body);
      if (!parsed.success) {
        return reply.code(400).send({ error: parsed.error.issues });
      }

      const plan = await findOwnedStudyPlanForComplete(planId, session.user.id);
      if (!plan) {
        return reply.code(404).send({ error: "Not found" });
      }
      if (plan.studyLog) {
        return reply
          .code(409)
          .send({ error: "この予定の実績はすでに記録されています" });
      }

      // 範囲は送られてきたものを優先し、無ければ予定の値をそのまま使う
      const rangeStart =
        parsed.data.rangeStart !== undefined ? parsed.data.rangeStart : plan.rangeStart;
      const rangeEnd =
        parsed.data.rangeEnd !== undefined ? parsed.data.rangeEnd : plan.rangeEnd;
      const rangeUnit =
        parsed.data.rangeUnit !== undefined ? parsed.data.rangeUnit : plan.rangeUnit;

      if (
        rangeEnd != null &&
        plan.textbook?.rangeUnit != null &&
        rangeUnit !== plan.textbook.rangeUnit
      ) {
        return reply
          .code(400)
          .send({ error: "範囲の単位を参考書の逆算設定に合わせてください" });
      }
      if (
        rangeEnd != null &&
        plan.textbook?.totalAmount != null &&
        rangeEnd > plan.textbook.totalAmount
      ) {
        return reply.code(400).send({
          error: `終了位置は参考書の総量（${plan.textbook.totalAmount}）以下にしてください`,
        });
      }

      try {
        const { log, updatedPlan, isFirstStudyLog } = await completeStudyPlan({
          userId: session.user.id,
          plan,
          minutes: parsed.data.minutes,
          rangeStart: rangeStart ?? null,
          rangeEnd: rangeEnd ?? null,
          rangeUnit: rangeUnit ?? null,
          memo: parsed.data.memo ?? null,
        });

        return reply.code(201).send({ log, plan: updatedPlan, isFirstStudyLog });
      } catch (error) {
        // 同じ予定を同時に完了すると一意制約に当たる。これは「すでに記録済み」なので 409。
        if (isDuplicateEntry(error)) {
          return reply
            .code(409)
            .send({ error: "この予定の実績はすでに記録されています" });
        }
        throw error;
      }
    }
  );
}
