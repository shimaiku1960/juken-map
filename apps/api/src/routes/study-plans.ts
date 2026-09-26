import type { FastifyInstance } from "fastify";
import { z } from "zod";
import { shiftYmd, todayYmdTokyo } from "@/shared/date";
import {
  createStudyPlansSchema,
  updateStudyPlanSchema,
} from "@/shared/validations/studyPlan";
import { completeStudyPlanSchema } from "@/shared/validations/studyLog";
import {
  completeOwnedStudyPlan,
  createStudyPlans,
  deleteStudyPlan,
  findOwnedStudyPlan,
  listStudyPlans,
  updateStudyPlan,
} from "@/api/services/study-plan-service";
import { countOwnedTextbooks } from "@/api/services/textbook-service";
import { currentSession } from "../access-control.ts";
import { readIdParam } from "./params.ts";

// 期間を省いて呼ばれたときの幅。予定は未来にもあるので前後に取る。画面はどれも
// 明示して呼ぶので、これは古いクライアントや手で叩いたときのための既定値。
const DEFAULT_PAST_DAYS = 90;
const DEFAULT_FUTURE_DAYS = 90;

const ymdField = z
  .string()
  .regex(/^\d{4}-\d{2}-\d{2}$/, "日付は YYYY-MM-DD で指定してください");

const rangeQuerySchema = z.object({
  from: ymdField.optional(),
  to: ymdField.optional(),
});

export function registerStudyPlanRoutes(app: FastifyInstance) {
  app.get("/api/study-plans", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const parsed = rangeQuerySchema.safeParse(request.query);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    // 今日は日本時間で決める（利用者も通知も日本時間で動いている）。
    const today = todayYmdTokyo();
    return listStudyPlans(session.user.id, {
      from: parsed.data.from ?? shiftYmd(today, -DEFAULT_PAST_DAYS),
      to: parsed.data.to ?? shiftYmd(today, DEFAULT_FUTURE_DAYS),
    });
  });

  app.post("/api/study-plans", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

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

  app.patch("/api/study-plans/:id", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const id = readIdParam(request.params, reply);
    if (id === null) return;

    const parsed = updateStudyPlanSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const plan = await findOwnedStudyPlan(id, session.user.id);
    if (!plan) {
      return reply.code(404).send({ error: "Not found" });
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

    const outcome = await updateStudyPlan(id, parsed.data);
    if (outcome.result === "has_log") {
      return reply
        .code(409)
        .send({ error: "実績を記録済みの予定は未完了に戻せません" });
    }
    return outcome.value;
  });

  app.delete("/api/study-plans/:id", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const id = readIdParam(request.params, reply);
    if (id === null) return;
    const plan = await findOwnedStudyPlan(id, session.user.id);
    if (!plan) {
      return reply.code(404).send({ error: "Not found" });
    }

    await deleteStudyPlan(id);
    return { message: "Deleted" };
  });

  app.post(
    "/api/study-plans/:id/complete",
    { config: { access: "user" } },
    async (request, reply) => {
      const session = currentSession(request);

      const planId = readIdParam(request.params, reply);
      if (planId === null) return;

      const parsed = completeStudyPlanSchema.safeParse(request.body);
      if (!parsed.success) {
        return reply.code(400).send({ error: parsed.error.issues });
      }

      const outcome = await completeOwnedStudyPlan({
        userId: session.user.id,
        planId,
        ...parsed.data,
      });
      switch (outcome.result) {
        case "not_found":
          return reply.code(404).send({ error: "Not found" });
        case "already_completed":
          return reply
            .code(409)
            .send({ error: "この予定の実績はすでに記録されています" });
        case "invalid_range":
          return reply.code(400).send({ error: outcome.message });
        case "ok":
          return reply.code(201).send(outcome.value);
      }
    }
  );
}
