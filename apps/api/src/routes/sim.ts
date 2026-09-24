import type { FastifyInstance, FastifyRequest } from "fastify";
import { z } from "zod";
import {
  getSimulationState,
  markSimulationUser,
  updateSimulationUser,
} from "@/api/services/simulation-service";
import { isSimEmail } from "@/shared/synthetic";
import { hasBearerToken } from "../bearer-token.ts";

// シミュレーション（sim/）専用の API。
//
// 守りは3重：
//   1. SIMULATION_ENABLED=on のときだけ登録する。付けなければ存在しない（404）。
//   2. SIMULATION_SECRET の Bearer が必須。未設定なら常に 401。
//   3. 触れる相手はシミュレーション用のメールアドレス（delivered+simNNNNN@resend.dev）だけ。
//
// 登録・確認メール・ログイン・学習記録などは、実際の利用者と同じ API と同じメールの経路を通る。
// ここにあるのは、シミュレーションの管理情報（連番・続き方の型・来なくなった日）の読み書きだけ。

const cohortSchema = z.enum(["steady", "fading", "sporadic", "dropped"]);
const ymd = z.string().regex(/^\d{4}-\d{2}-\d{2}$/);

const markBodySchema = z.object({
  email: z.string().email(),
  seq: z.number().int().positive(),
  cohort: cohortSchema,
});
const updateBodySchema = z.object({
  lastActedOn: ymd.nullable().optional(),
  dormantFrom: ymd.nullable().optional(),
});

function isAuthorized(request: FastifyRequest) {
  return hasBearerToken(request, process.env.SIMULATION_SECRET);
}

export function registerSimRoutes(app: FastifyInstance) {
  if (process.env.SIMULATION_ENABLED !== "on") return;

  app.addHook("onRequest", async (request, reply) => {
    if (!request.url.startsWith("/api/sim/")) return;
    if (!isAuthorized(request)) {
      return reply.code(401).send({ error: "Unauthorized" });
    }
  });

  app.get("/api/sim/state", async () => getSimulationState());

  app.post("/api/sim/users", async (request, reply) => {
    const parsed = markBodySchema.safeParse(request.body);
    if (!parsed.success || !isSimEmail(parsed.data.email)) {
      return reply.code(400).send({ error: "入力が不正です" });
    }
    const { email, ...data } = parsed.data;
    const result = await markSimulationUser(email, data);
    if (result === "not_found") return reply.code(404).send({ error: "ユーザーが見つかりません" });
    if (result === "duplicate") return reply.code(409).send({ error: "この連番はすでに使われています" });
    return reply.code(204).send();
  });

  app.patch<{ Params: { seq: string } }>("/api/sim/users/:seq", async (request, reply) => {
    const seq = Number(request.params.seq);
    const parsed = updateBodySchema.safeParse(request.body);
    if (!Number.isInteger(seq) || seq <= 0 || !parsed.success) {
      return reply.code(400).send({ error: "入力が不正です" });
    }
    const found = await updateSimulationUser(seq, parsed.data);
    if (!found) return reply.code(404).send({ error: "ユーザーが見つかりません" });
    return reply.code(204).send();
  });
}
