import type { FastifyInstance } from "fastify";
import { findSignUpMethod, markSignUpTracked } from "@/api/services/user-service";
import { currentSession } from "../access-control.ts";

export function registerAnalyticsRoutes(app: FastifyInstance) {
  app.post("/api/analytics/registration", { config: { access: "user" } }, async (request) => {
    const session = currentSession(request);

    // 初回だけ計測を飛ばす。2回目以降は shouldTrack: false で黙って終わる。
    const isFirstTime = await markSignUpTracked(session.user.id);
    if (!isFirstTime) {
      return { shouldTrack: false };
    }

    return { shouldTrack: true, method: await findSignUpMethod(session.user.id) };
  });
}
