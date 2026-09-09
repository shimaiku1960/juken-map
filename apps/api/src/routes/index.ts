import type { FastifyInstance } from "fastify";
import { registerAnalyticsRoutes } from "./analytics.ts";
import { registerCronRoutes } from "./cron.ts";
import { registerGoalRoutes } from "./goals.ts";
import { registerHomeRoutes } from "./home.ts";
import { registerLineRoutes } from "./line.ts";
import { registerNotificationPreferenceRoutes } from "./notification-preferences.ts";
import { registerProfileRoutes } from "./profile.ts";
import { registerStudyLogItemRoutes } from "./study-log-item.ts";
import { registerStudyLogRoutes } from "./study-logs.ts";
import { registerStudyPlanRoutes } from "./study-plans.ts";
import { registerTextbookMasterRoutes } from "./textbook-masters.ts";
import { registerTextbookRoutes } from "./textbooks.ts";
import { registerUniversityRoutes } from "./universities.ts";

export function registerRoutes(app: FastifyInstance) {
  registerAnalyticsRoutes(app);
  registerCronRoutes(app);
  registerGoalRoutes(app);
  registerHomeRoutes(app);
  registerLineRoutes(app);
  registerNotificationPreferenceRoutes(app);
  registerProfileRoutes(app);
  registerStudyLogRoutes(app);
  registerStudyLogItemRoutes(app);
  registerStudyPlanRoutes(app);
  registerTextbookMasterRoutes(app);
  registerTextbookRoutes(app);
  registerUniversityRoutes(app);
}
