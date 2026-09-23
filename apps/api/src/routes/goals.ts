import type { FastifyInstance } from "fastify";
import { goalSchema, updateGoalSchema, patchGoalSchema } from "@/shared/validations/goal";
import {
  applyGoalPatch,
  createGoal,
  deleteGoal,
  findOwnedGoal,
  listGoals,
  updateGoal,
} from "@/api/services/goal-service";
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

    const outcome = await createGoal({
      userId: session.user.id,
      facultyId: parsed.data.facultyId,
      status: parsed.data.status,
    });
    if (outcome.result === "duplicate") {
      return reply.code(409).send({ error: "この学部はすでに登録されています" });
    }
    return reply.code(201).send(outcome.value);
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
    const goal = await findOwnedGoal(id, session.user.id);
    if (!goal) {
      return reply.code(404).send({ error: "Not found" });
    }

    return updateGoal(id, parsed.data);
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
    const goal = await findOwnedGoal(id, session.user.id);
    if (!goal) {
      return reply.code(404).send({ error: "Not found" });
    }

    await applyGoalPatch(session.user.id, id, parsed.data);
    return { message: "OK" };
  });

  app.delete<{ Params: IdParams }>("/api/goals/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const id = Number(request.params.id);
    const goal = await findOwnedGoal(id, session.user.id);
    if (!goal) {
      return reply.code(404).send({ error: "Not found" });
    }

    await deleteGoal(id);
    return { message: "Deleted" };
  });
}
