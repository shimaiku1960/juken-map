import type { FastifyInstance } from "fastify";
import { profileSchema } from "@/shared/validations/profile";
import { updateProfile } from "@/api/services/user-service";
import { currentSession } from "../access-control.ts";

export function registerProfileRoutes(app: FastifyInstance) {
  app.put("/api/profile", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const result = profileSchema.safeParse(request.body);
    if (!result.success) {
      return reply.code(400).send({ error: result.error.issues[0].message });
    }

    return updateProfile(session.user.id, { nickname: result.data.nickname });
  });
}
