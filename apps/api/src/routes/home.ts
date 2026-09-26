import type { FastifyInstance } from "fastify";
import { findFirstChoiceGoal } from "@/api/services/goal-service";
import { currentSession } from "../access-control.ts";

// トップ画面が表示する「第一志望」。Next.js では Server Component が直接呼んでいた。
export function registerHomeRoutes(app: FastifyInstance) {
  app.get("/api/goals/first-choice", { config: { access: "user" } }, async (request) => {
    const session = currentSession(request);

    return findFirstChoiceGoal(session.user.id);
  });
}
