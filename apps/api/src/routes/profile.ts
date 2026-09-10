import type { FastifyInstance } from "fastify";
import { profileSchema } from "@/shared/validations/profile";
import { updateProfile } from "@/api/services/user-service";
import { denyDemoWrite, requireSession } from "../context.ts";

export function registerProfileRoutes(app: FastifyInstance) {
  app.put("/api/profile", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const result = profileSchema.safeParse(request.body);
    if (!result.success) {
      return reply.code(400).send({ error: result.error.issues[0].message });
    }

    return updateProfile(session.user.id, { nickname: result.data.nickname });
  });
}
