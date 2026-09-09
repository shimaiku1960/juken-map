import type { FastifyInstance } from "fastify";
import { listTextbookMasters } from "@/backend/services/textbook-service";
import { requireSession } from "../context.ts";

export function registerTextbookMasterRoutes(app: FastifyInstance) {
  app.get("/api/textbook-masters", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    return listTextbookMasters();
  });
}
