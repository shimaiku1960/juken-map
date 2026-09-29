import type { FastifyInstance } from "fastify";
import { profileSchema } from "@/shared/validations/profile";
import { updateProfile } from "@/api/services/user-service";
import { currentSession } from "../access-control.ts";
import { sendValidationError } from "./validation-error.ts";

export function registerProfileRoutes(app: FastifyInstance) {
  app.put("/api/profile", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const result = profileSchema.safeParse(request.body);
    if (!result.success) {
      return sendValidationError(reply, result.error);
    }

    return updateProfile(session.user.id, { nickname: result.data.nickname });
  });
}
