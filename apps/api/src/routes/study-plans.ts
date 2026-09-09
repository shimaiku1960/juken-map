import type { FastifyInstance } from "fastify";
import { prisma } from "@/backend/infra/prisma";
import { Prisma } from "@/app/generated/prisma/client";
import {
  createStudyPlansSchema,
  updateStudyPlanSchema,
} from "@/shared/validations/studyPlan";
import { completeStudyPlanSchema } from "@/shared/validations/studyLog";
import { toStudyPlanDTO } from "@/backend/dto/study-mapper";
import { listStudyPlans } from "@/backend/services/study-plan-service";
import { denyDemoWrite, requireSession } from "../context.ts";

type IdParams = { id: string };

export function registerStudyPlanRoutes(app: FastifyInstance) {
  app.get("/api/study-plans", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    const plans = await listStudyPlans(session.user.id);
    return plans.map(toStudyPlanDTO);
  });

  app.post("/api/study-plans", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = createStudyPlansSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const date = new Date(parsed.data.date);

    // 参考書は他人のIDを混ぜられないよう、自分の所有分だけを許可する
    const textbookIds = [
      ...new Set(
        parsed.data.items
          .map((item) => item.textbookId)
          .filter((id): id is number => id != null)
      ),
    ];

    if (textbookIds.length > 0) {
      const owned = await prisma.textbook.count({
        where: { id: { in: textbookIds }, userId: session.user.id },
      });
      if (owned !== textbookIds.length) {
        return reply.code(400).send({ error: "不正な参考書が含まれています" });
      }
    }

    // 1つの日付に複数の内容をまとめて作成
    const result = await prisma.studyPlan.createMany({
      data: parsed.data.items.map((item) => ({
        date,
        textbookId: item.textbookId ?? null,
        rangeStart: item.rangeStart ?? null,
        rangeEnd: item.rangeEnd ?? null,
        rangeUnit: item.rangeUnit ?? null,
        content: item.content ?? null,
        subject: item.subject ?? null,
        userId: session.user.id,
      })),
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
    const plan = await prisma.studyPlan.findUnique({ where: { id } });
    if (!plan || plan.userId !== session.user.id) {
      return reply.code(404).send({ error: "Not found" });
    }

    if (parsed.data.done === false) {
      const linkedLog = await prisma.studyLog.count({
        where: { studyPlanId: plan.id },
      });
      if (linkedLog > 0) {
        return reply
          .code(409)
          .send({ error: "実績を記録済みの予定は未完了に戻せません" });
      }
    }

    // 参考書を指定する場合は、自分の所有分だけを許可する
    if (parsed.data.textbookId != null) {
      const owned = await prisma.textbook.count({
        where: { id: parsed.data.textbookId, userId: session.user.id },
      });
      if (owned === 0) {
        return reply.code(400).send({ error: "不正な参考書です" });
      }
    }

    return prisma.studyPlan.update({
      where: { id },
      data: {
        ...(parsed.data.date && { date: new Date(parsed.data.date) }),
        ...(parsed.data.content !== undefined && { content: parsed.data.content }),
        ...(parsed.data.subject !== undefined && { subject: parsed.data.subject }),
        ...(parsed.data.textbookId !== undefined && {
          textbookId: parsed.data.textbookId,
        }),
        ...(parsed.data.rangeStart !== undefined && {
          rangeStart: parsed.data.rangeStart,
        }),
        ...(parsed.data.rangeEnd !== undefined && { rangeEnd: parsed.data.rangeEnd }),
        ...(parsed.data.rangeUnit !== undefined && {
          rangeUnit: parsed.data.rangeUnit,
        }),
        ...(parsed.data.done !== undefined && { done: parsed.data.done }),
      },
    });
  });

  app.delete<{ Params: IdParams }>("/api/study-plans/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const id = Number(request.params.id);
    const plan = await prisma.studyPlan.findUnique({ where: { id } });
    if (!plan || plan.userId !== session.user.id) {
      return reply.code(404).send({ error: "Not found" });
    }

    await prisma.studyPlan.delete({ where: { id } });
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

      const plan = await prisma.studyPlan.findFirst({
        where: { id: planId, userId: session.user.id },
        include: { textbook: true, studyLog: true },
      });
      if (!plan) {
        return reply.code(404).send({ error: "Not found" });
      }
      if (plan.studyLog) {
        return reply
          .code(409)
          .send({ error: "この予定の実績はすでに記録されています" });
      }

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
        const { log, updatedPlan, isFirstStudyLog } = await prisma.$transaction(
          async (tx) => {
            const activation = await tx.user.updateMany({
              where: { id: session.user.id, firstStudyLogAt: null },
              data: { firstStudyLogAt: new Date() },
            });
            const log = await tx.studyLog.create({
              data: {
                userId: session.user.id,
                studyPlanId: plan.id,
                date: plan.date,
                minutes: parsed.data.minutes,
                subject: plan.subject,
                textbookId: plan.textbookId,
                rangeStart,
                rangeEnd,
                rangeUnit,
                memo: parsed.data.memo ?? null,
              },
              include: { textbook: true },
            });
            const updatedPlan = await tx.studyPlan.update({
              where: { id: plan.id },
              data: { done: true },
            });
            return { log, updatedPlan, isFirstStudyLog: activation.count === 1 };
          }
        );

        return reply.code(201).send({ log, plan: updatedPlan, isFirstStudyLog });
      } catch (error) {
        if (
          error instanceof Prisma.PrismaClientKnownRequestError &&
          error.code === "P2002"
        ) {
          return reply
            .code(409)
            .send({ error: "この予定の実績はすでに記録されています" });
        }
        throw error;
      }
    }
  );
}
