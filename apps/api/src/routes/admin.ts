import type { FastifyInstance, FastifyReply, FastifyRequest } from "fastify";
import { z } from "zod";
import {
  banUser,
  deleteUser,
  getAdminOverview,
  listAdminUsers,
  unbanUser,
  type ProtectedReason,
  type UserActionOutcome,
} from "@/api/services/admin-service";
import { USER_KINDS } from "@/shared/dto/admin";
import { currentSession } from "../access-control.ts";

// 管理者ページ（/admin）の API。どれも access: "admin"（role = 'admin' だけ）。
// 画面側でもメニューを出し分けているが、守るのはサーバー側（access-control.ts）。

const listUsersQuerySchema = z.object({
  kind: z.enum(USER_KINDS).default("real"),
  q: z.string().max(191).optional(),
  page: z.coerce.number().int().positive().max(10_000).default(1),
});

const userIdParamsSchema = z.object({ id: z.string().min(1).max(191) });

// 削除は取り消せないので、画面で打ち込んだメールアドレスを本人のものと突き合わせる。
const deleteUserBodySchema = z.object({ email: z.string().min(1).max(191) });

const PROTECTED_MESSAGES: Record<ProtectedReason, string> = {
  self: "自分自身は停止・削除できません",
  admin: "他の管理者は停止・削除できません（先に権限を外してください）",
  demo: "デモアカウントは停止・削除できません",
};

/**
 * 操作できなかった理由を HTTP の返事にする。
 * 404 は相手がいない、409 は相手はいるが守られている、400 は確認のメールアドレス違い。
 */
function sendUserActionFailure(
  reply: FastifyReply,
  outcome: Exclude<UserActionOutcome<unknown>, { result: "ok" }>
) {
  switch (outcome.result) {
    case "not_found":
      return reply.code(404).send({ error: "ユーザーが見つかりません" });
    case "protected":
      return reply.code(409).send({ error: PROTECTED_MESSAGES[outcome.reason] });
    case "email_mismatch":
      return reply.code(400).send({ error: "メールアドレスが一致しません" });
  }
}

// 誰が・誰に・何をしたかを構造化ログに残す（Grafana の Loki で追える）。
// マスター編集の `admin master change` と同じ考え方で、専用テーブルは作らない。
function logUserAction(
  request: FastifyRequest,
  adminId: string,
  action: "ban" | "unban" | "delete",
  target: { id: string; email: string | null },
  detail?: Record<string, unknown>
) {
  request.log.warn({ adminId, action, targetId: target.id, targetEmail: target.email, ...detail }, "admin user action");
}

export function registerAdminRoutes(app: FastifyInstance) {
  app.get("/api/admin/overview", { config: { access: "admin" } }, async () => {
    return getAdminOverview();
  });

  app.get("/api/admin/users", { config: { access: "admin" } }, async (request, reply) => {
    const parsed = listUsersQuerySchema.safeParse(request.query);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    return listAdminUsers(parsed.data);
  });

  app.post("/api/admin/users/:id/ban", { config: { access: "admin" } }, async (request, reply) => {
    const session = currentSession(request);

    const params = userIdParamsSchema.safeParse(request.params);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });

    const outcome = await banUser(params.data.id, session.user.id);
    if (outcome.result !== "ok") return sendUserActionFailure(reply, outcome);

    logUserAction(request, session.user.id, "ban", outcome.value, {
      sessionsRemoved: outcome.value.sessionsRemoved,
    });
    return outcome.value;
  });

  app.post("/api/admin/users/:id/unban", { config: { access: "admin" } }, async (request, reply) => {
    const session = currentSession(request);

    const params = userIdParamsSchema.safeParse(request.params);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });

    const outcome = await unbanUser(params.data.id);
    if (outcome.result !== "ok") return sendUserActionFailure(reply, outcome);

    logUserAction(request, session.user.id, "unban", outcome.value);
    return outcome.value;
  });

  app.delete("/api/admin/users/:id", { config: { access: "admin" } }, async (request, reply) => {
    const session = currentSession(request);

    const params = userIdParamsSchema.safeParse(request.params);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });
    const body = deleteUserBodySchema.safeParse(request.body);
    if (!body.success) return reply.code(400).send({ error: body.error.issues });

    const outcome = await deleteUser(params.data.id, session.user.id, body.data.email);
    if (outcome.result !== "ok") return sendUserActionFailure(reply, outcome);

    logUserAction(request, session.user.id, "delete", outcome.value, { removed: outcome.value.removed });
    return outcome.value;
  });
}
