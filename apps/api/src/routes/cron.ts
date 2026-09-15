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

    const summary = await sendDailyNotifications(result.data.slot);
    // 1回の実行で何通送れて何通失敗したかを残す。失敗が続いていないかを後から追える。
    request.log.info({ notification: summary }, "[daily-notification] Run finished.");
    return summary;
  });
}
