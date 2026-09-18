import type { FastifyInstance } from "fastify";
import { z } from "zod";
import { getAdminOverview, listAdminUsers } from "@/api/services/admin-service";
import { USER_KINDS } from "@/shared/dto/admin";
import { requireAdmin } from "../context.ts";

// 管理者ページ（/admin）の API。どれも requireAdmin を通す（role = 'admin' だけ）。
// 画面側でもメニューを出し分けているが、守るのはここ。

const listUsersQuerySchema = z.object({
  kind: z.enum(USER_KINDS).default("real"),
  q: z.string().max(191).optional(),
  page: z.coerce.number().int().positive().max(10_000).default(1),
});

export function registerAdminRoutes(app: FastifyInstance) {
  app.get("/api/admin/overview", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;

    return getAdminOverview();
  });

  app.get("/api/admin/users", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;

    const parsed = listUsersQuerySchema.safeParse(request.query);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    return listAdminUsers(parsed.data);
  });
}
