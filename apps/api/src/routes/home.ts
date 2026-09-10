import type { FastifyInstance } from "fastify";
import { findFirstChoiceGoal } from "@/api/services/goal-service";
import { requireSession } from "../context.ts";

// トップ画面が表示する「第一志望」。Next.js では Server Component が直接呼んでいた。
export function registerHomeRoutes(app: FastifyInstance) {
  app.get("/api/goals/first-choice", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    return findFirstChoiceGoal(session.user.id);
  });
}
