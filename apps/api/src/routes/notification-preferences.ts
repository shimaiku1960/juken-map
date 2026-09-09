import type { FastifyInstance } from "fastify";
import { prisma } from "@/backend/infra/prisma";
import {
  findLineConnection,
  findNotificationPreference,
} from "@/backend/services/notification-service";
import { notificationPreferenceSchema } from "@/shared/validations/notification";
import { denyDemoWrite, requireSession } from "../context.ts";

const DEFAULT_PREFERENCE = {
  emailMorningEnabled: false,
  emailEveningEnabled: false,
  lineMorningEnabled: false,
  lineEveningEnabled: false,
};

export function registerNotificationPreferenceRoutes(app: FastifyInstance) {
  app.get("/api/notification-preferences", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    const preference = await findNotificationPreference(session.user.id);
    return preference
      ? {
          emailMorningEnabled: preference.morningEnabled,
          emailEveningEnabled: preference.eveningEnabled,
          lineMorningEnabled: preference.lineMorningEnabled,
          lineEveningEnabled: preference.lineEveningEnabled,
        }
      : DEFAULT_PREFERENCE;
  });

  app.put("/api/notification-preferences", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const result = notificationPreferenceSchema.safeParse(request.body);
    if (!result.success) {
      return reply.code(400).send({ error: result.error.issues[0].message });
    }

    if (result.data.lineMorningEnabled || result.data.lineEveningEnabled) {
      const connection = await findLineConnection(session.user.id);
      if (!connection) {
        return reply
          .code(400)
          .send({ error: "LINEと連携してからLINE通知を選択してください" });
      }
    }

    const data = {
      morningEnabled: result.data.emailMorningEnabled,
      eveningEnabled: result.data.emailEveningEnabled,
      lineMorningEnabled: result.data.lineMorningEnabled,
      lineEveningEnabled: result.data.lineEveningEnabled,
    };

    const preference = await prisma.notificationPreference.upsert({
      where: { userId: session.user.id },
      create: { userId: session.user.id, ...data },
      update: data,
      select: {
        morningEnabled: true,
        eveningEnabled: true,
        lineMorningEnabled: true,
        lineEveningEnabled: true,
      },
    });

    return {
      emailMorningEnabled: preference.morningEnabled,
      emailEveningEnabled: preference.eveningEnabled,
      lineMorningEnabled: preference.lineMorningEnabled,
      lineEveningEnabled: preference.lineEveningEnabled,
    };
  });
}
