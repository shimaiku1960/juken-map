import type { FastifyInstance, FastifyRequest } from "fastify";
import { z } from "zod";
import { sendDailyNotifications } from "@/api/services/sendDailyNotifications";

const bodySchema = z.object({ slot: z.enum(["morning", "evening"]) });

// 認証はセッションではなく共有シークレット。GitHub Actions から呼ばれる。
function isAuthorized(request: FastifyRequest) {
  const secret = process.env.DAILY_NOTIFICATION_SECRET;
  return Boolean(secret) && request.headers.authorization === `Bearer ${secret}`;
}

export function registerCronRoutes(app: FastifyInstance) {
  app.post("/api/cron/daily-study-notifications", async (request, reply) => {
    if (!isAuthorized(request)) {
      return reply.code(401).send({ error: "Unauthorized" });
    }

    const result = bodySchema.safeParse(request.body);
    if (!result.success) {
      return reply.code(400).send({ error: "Invalid slot" });
    }

    return sendDailyNotifications(result.data.slot);
  });
}
