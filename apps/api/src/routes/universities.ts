import type { FastifyInstance } from "fastify";
import {
  findUniversityDetail,
  listUniversitiesForExplore,
} from "@/api/services/university-service";
import {
  findFirstChoiceGoal,
  listGoalFacultyIds,
} from "@/api/services/goal-service";
import { requireSession } from "../context.ts";

// Next.js では Server Component がサービス層を関数として直接呼んでいたため、
// これらに HTTP の入口が無かった。SPA からは HTTP でしか取れないので新設する。
// 分離して初めて「どこまでが本当に API だったか」が可視化された部分。
export function registerUniversityRoutes(app: FastifyInstance) {
  app.get("/api/universities", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    return listUniversitiesForExplore();
  });

  app.get<{ Params: { id: string } }>(
    "/api/universities/:id",
    async (request, reply) => {
      const session = await requireSession(request, reply);
      if (!session) return;

      const id = Number(request.params.id);
      if (!Number.isInteger(id)) {
        return reply.code(400).send({ error: "Invalid university id" });
      }

      const university = await findUniversityDetail(id);
      if (!university) {
        return reply.code(404).send({ error: "Not found" });
      }

      // 画面は「この学部は登録済みか」を出し分けるので、同じ応答に含める。
      // 別々に取ると往復が増えるうえ、画面側で組み合わせる責務が生まれる。
      const goals = await listGoalFacultyIds(session.user.id);
      return {
        university,
        registeredFacultyIds: goals.map((g) => g.facultyId),
      };
    }
  );
}
