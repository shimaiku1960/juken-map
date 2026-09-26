import type { FastifyInstance } from "fastify";
import {
  findUniversityDetail,
  getUniversitiesForExplore,
} from "@/api/services/university-service";
import { listGoalFacultyIds } from "@/api/services/goal-service";
import { currentSession } from "../access-control.ts";
import { readIdParam } from "./params.ts";

// If-None-Match は複数の値をカンマで並べられる。途中で nginx などが圧縮し直すと
// ETag は弱い形（W/"..."）に書き換わって戻ってくるので、W/ を外して比べる。
function matchesEtag(ifNoneMatch: string | undefined, etag: string) {
  if (!ifNoneMatch) return false;
  return ifNoneMatch
    .split(",")
    .some((value) => value.trim().replace(/^W\//, "") === etag);
}

/**
 * Accept-Encoding から、手元にある圧縮版（br / gzip）のうち返してよいものを選ぶ。
 * どちらも受け付けなければ null（圧縮しないで返す）。
 *
 * "br;q=0" のように q=0 が付いたものは「受け付けない」の意味なので除く。
 * 受け付けるものの中で q の大小までは見ず、圧縮率の高い br を優先する。
 */
export function pickEncoding(acceptEncoding: string | undefined): "br" | "gzip" | null {
  if (!acceptEncoding) return null;
  const accepted = new Set<string>();
  for (const part of acceptEncoding.split(",")) {
    const [name, ...params] = part.trim().toLowerCase().split(";");
    const q = params.map((p) => p.trim()).find((p) => p.startsWith("q="));
    if (q !== undefined && Number(q.slice(2)) === 0) continue;
    accepted.add(name.trim());
  }
  if (accepted.has("br") || accepted.has("*")) return "br";
  if (accepted.has("gzip")) return "gzip";
  return null;
}

// Next.js では Server Component がサービス層を関数として直接呼んでいたため、
// これらに HTTP の入口が無かった。SPA からは HTTP でしか取れないので新設する。
// 分離して初めて「どこまでが本当に API だったか」が可視化された部分。
export function registerUniversityRoutes(app: FastifyInstance) {
  app.get("/api/universities", { config: { access: "user" } }, async (request, reply) => {
    // 中身は全員共通で、変わるのは管理画面の編集だけ。ブラウザには毎回確かめさせ（no-cache）、
    // 変わっていなければ 304 で本文を省く。ログインが要る応答なので共有キャッシュには置かせない。
    const snapshot = await getUniversitiesForExplore();
    reply.header("Cache-Control", "private, no-cache");
    reply.header("ETag", snapshot.etag);
    // 同じ URL でも Accept-Encoding で本文の形が変わる、と途中のキャッシュに伝える。
    reply.header("Vary", "Accept-Encoding");
    if (matchesEtag(request.headers["if-none-match"], snapshot.etag)) {
      return reply.code(304).send();
    }
    reply.type("application/json; charset=utf-8");
    // 圧縮済みの形をそのまま返す。Content-Encoding が付いていれば @fastify/compress は
    // 圧縮し直さない（二重に圧縮されない）。
    const encoding = pickEncoding(request.headers["accept-encoding"]);
    if (encoding) {
      return reply.header("Content-Encoding", encoding).send(snapshot[encoding]);
    }
    return reply.send(snapshot.json);
  });

  app.get(
    "/api/universities/:id",
    { config: { access: "user" } },
    async (request, reply) => {
      const session = currentSession(request);

      const id = readIdParam(request.params, reply);
      if (id === null) return;

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
