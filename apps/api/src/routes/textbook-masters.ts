import type { FastifyInstance } from "fastify";
import { listTextbookMasters } from "@/api/services/textbook-service";

export function registerTextbookMasterRoutes(app: FastifyInstance) {
  app.get("/api/textbook-masters", { config: { access: "user" } }, async () => {
    return listTextbookMasters();
  });
}
