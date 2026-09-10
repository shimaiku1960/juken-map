import type { FastifyInstance } from "fastify";
import { prisma } from "@/api/infra/prisma";
import { requireSession } from "../context.ts";

export function registerAnalyticsRoutes(app: FastifyInstance) {
  app.post("/api/analytics/registration", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    const tracked = await prisma.user.updateMany({
      where: { id: session.user.id, analyticsSignUpTrackedAt: null },
      data: { analyticsSignUpTrackedAt: new Date() },
    });
    if (tracked.count === 0) {
      return { shouldTrack: false };
    }

    const account = await prisma.account.findFirst({
      where: { userId: session.user.id },
      select: { providerId: true },
      orderBy: { createdAt: "asc" },
    });
    const method =
      account?.providerId === "google" || account?.providerId === "github"
        ? account.providerId
        : "email";

    return { shouldTrack: true, method };
  });
}
