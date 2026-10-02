import type { FastifyInstance } from "fastify";
import { registerBlogRoutes } from "./blog.ts";
import { registerLineRoutes } from "./line.ts";

export function registerRoutes(app: FastifyInstance) {
  registerBlogRoutes(app);
  registerLineRoutes(app);
}
