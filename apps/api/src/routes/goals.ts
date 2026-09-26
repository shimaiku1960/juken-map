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
import { currentSession } from "../access-control.ts";
import { readIdParam } from "./params.ts";

export function registerGoalRoutes(app: FastifyInstance) {
  app.get("/api/goals", { config: { access: "user" } }, async (request) => {
    const session = currentSession(request);

    return listGoals(session.user.id);
  });

  app.post("/api/goals", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

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

  app.put("/api/goals/:id", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const id = readIdParam(request.params, reply);
    if (id === null) return;

    const parsed = updateGoalSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const goal = await findOwnedGoal(id, session.user.id);
    if (!goal) {
      return reply.code(404).send({ error: "Not found" });
    }

    return updateGoal(id, parsed.data);
  });

  app.patch("/api/goals/:id", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const id = readIdParam(request.params, reply);
    if (id === null) return;

    const parsed = patchGoalSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const goal = await findOwnedGoal(id, session.user.id);
    if (!goal) {
      return reply.code(404).send({ error: "Not found" });
    }

    await applyGoalPatch(session.user.id, id, parsed.data);
    return { message: "OK" };
  });

  app.delete("/api/goals/:id", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const id = readIdParam(request.params, reply);
    if (id === null) return;
    const goal = await findOwnedGoal(id, session.user.id);
    if (!goal) {
      return reply.code(404).send({ error: "Not found" });
    }

    await deleteGoal(id);
    return { message: "Deleted" };
  });
}
