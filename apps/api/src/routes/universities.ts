import type { FastifyInstance } from "fastify";
import {
  findUniversityDetail,
  getUniversitiesForExplore,
} from "@/api/services/university-service";
import { listGoalFacultyIds } from "@/api/services/goal-service";
import { requireSession } from "../context.ts";

// If-None-Match は複数の値をカンマで並べられる。途中で nginx などが圧縮し直すと
// ETag は弱い形（W/"..."）に書き換わって戻ってくるので、W/ を外して比べる。
function matchesEtag(ifNoneMatch: string | undefined, etag: string) {
  if (!ifNoneMatch) return false;
  return ifNoneMatch
    .split(",")
    .some((value) => value.trim().replace(/^W\//, "") === etag);
}

// Next.js では Server Component がサービス層を関数として直接呼んでいたため、
// これらに HTTP の入口が無かった。SPA からは HTTP でしか取れないので新設する。
// 分離して初めて「どこまでが本当に API だったか」が可視化された部分。
export function registerUniversityRoutes(app: FastifyInstance) {
  app.get("/api/universities", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    // 中身は全員共通で、変わるのは管理画面の編集だけ。ブラウザには毎回確かめさせ（no-cache）、
    // 変わっていなければ 304 で本文を省く。ログインが要る応答なので共有キャッシュには置かせない。
    const { json, etag } = await getUniversitiesForExplore();
    reply.header("Cache-Control", "private, no-cache");
    reply.header("ETag", etag);
    if (matchesEtag(request.headers["if-none-match"], etag)) {
      return reply.code(304).send();
    }
    return reply.type("application/json; charset=utf-8").send(json);
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
