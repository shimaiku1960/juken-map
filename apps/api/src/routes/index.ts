import type { FastifyInstance } from "fastify";
import { registerAdminRoutes } from "./admin.ts";
import { registerAdminMasterRoutes } from "./admin-masters.ts";
import { registerBlogRoutes } from "./blog.ts";
import { registerGoalRoutes } from "./goals.ts";
import { registerHomeRoutes } from "./home.ts";
import { registerLineRoutes } from "./line.ts";
import { registerStudyPlanRoutes } from "./study-plans.ts";
import { registerTextbookRoutes } from "./textbooks.ts";

export function registerRoutes(app: FastifyInstance) {
  registerAdminRoutes(app);
  registerAdminMasterRoutes(app);
  registerBlogRoutes(app);
  registerGoalRoutes(app);
  registerHomeRoutes(app);
  registerLineRoutes(app);
  registerStudyPlanRoutes(app);
  registerTextbookRoutes(app);
}
