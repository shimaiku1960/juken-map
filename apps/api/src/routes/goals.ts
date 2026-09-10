import type { FastifyInstance } from "fastify";
import { prisma } from "@/api/infra/prisma";
import { Prisma } from "@/api/generated/prisma/client";
import { goalSchema, updateGoalSchema, patchGoalSchema } from "@/shared/validations/goal";
import { listGoals } from "@/api/services/goal-service";
import { denyDemoWrite, requireSession } from "../context.ts";

type IdParams = { id: string };

export function registerGoalRoutes(app: FastifyInstance) {
  app.get("/api/goals", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    return listGoals(session.user.id);
  });

  app.post("/api/goals", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = goalSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    try {
      const goal = await prisma.finalGoal.create({
        data: {
          facultyId: parsed.data.facultyId,
          userId: session.user.id,
          status: parsed.data.status ?? "decided",
        },
        include: {
          faculty: {
            include: { university: true },
          },
        },
      });
      return reply.code(201).send(goal);
    } catch (error) {
      if (
        error instanceof Prisma.PrismaClientKnownRequestError &&
        error.code === "P2002"
      ) {
        return reply.code(409).send({ error: "この学部はすでに登録されています" });
      }
      throw error;
    }
  });

  app.put<{ Params: IdParams }>("/api/goals/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = updateGoalSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const id = Number(request.params.id);
    const goal = await prisma.finalGoal.findUnique({ where: { id } });
    if (!goal || goal.userId !== session.user.id) {
      return reply.code(404).send({ error: "Not found" });
    }

    return prisma.finalGoal.update({
      where: { id },
      data: {
        ...(parsed.data.facultyId && { facultyId: parsed.data.facultyId }),
      },
    });
  });

  app.patch<{ Params: IdParams }>("/api/goals/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = patchGoalSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const id = Number(request.params.id);
    const goal = await prisma.finalGoal.findUnique({ where: { id } });
    if (!goal || goal.userId !== session.user.id) {
      return reply.code(404).send({ error: "Not found" });
    }

    // 第一志望トグル（送られてきたときだけ処理。未指定なら現状維持）
    if (parsed.data.isFirstChoice !== undefined) {
      if (parsed.data.isFirstChoice) {
        // 第一志望は1ユーザー1校まで。既存の第一志望を全部外してから付け替える
        await prisma.$transaction([
          prisma.finalGoal.updateMany({
            where: { userId: session.user.id },
            data: { isFirstChoice: false },
          }),
          prisma.finalGoal.update({
            where: { id },
            data: { isFirstChoice: true },
          }),
        ]);
      } else {
        await prisma.finalGoal.update({
          where: { id },
          data: { isFirstChoice: false },
        });
      }
    }

    // メモ更新（送られてきたときだけ）
    if (parsed.data.note !== undefined) {
      await prisma.finalGoal.update({ where: { id }, data: { note: parsed.data.note } });
    }

    // ステータス更新（候補→受験校に確定 など。送られてきたときだけ）
    if (parsed.data.status !== undefined) {
      await prisma.finalGoal.update({ where: { id }, data: { status: parsed.data.status } });
    }

    return { message: "OK" };
  });

  app.delete<{ Params: IdParams }>("/api/goals/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const id = Number(request.params.id);
    const goal = await prisma.finalGoal.findUnique({ where: { id } });
    if (!goal || goal.userId !== session.user.id) {
      return reply.code(404).send({ error: "Not found" });
    }

    await prisma.finalGoal.delete({ where: { id } });
    return { message: "Deleted" };
  });
}
