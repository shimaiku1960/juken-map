import type { FastifyInstance, FastifyReply, FastifyRequest } from "fastify";
import { z } from "zod";
import {
  createFaculty,
  createTextbookMaster,
  createUniversity,
  deleteFaculty,
  deleteTextbookMaster,
  deleteUniversity,
  getAdminUniversityDetail,
  listAdminTags,
  listAdminTextbookMasters,
  listAdminUniversities,
  updateFaculty,
  updateTextbookMaster,
  updateUniversity,
} from "@/api/services/master-service";
import {
  createFacultySchema,
  facultyInputSchema,
  textbookMasterInputSchema,
  universityInputSchema,
} from "@/shared/validations/master";
import { requireAdmin } from "../context.ts";

// 管理者ページのマスター編集（/admin/masters、大学・学部・参考書）の API。どれも requireAdmin を通す。
// 変更はすべて構造化ログ `admin master change` に「誰が・何を・前→後」で残す（Grafana の Loki で追える）。

const listQuerySchema = z.object({
  q: z.string().trim().max(100).optional(),
  page: z.coerce.number().int().positive().max(1_000).default(1),
});
const searchQuerySchema = z.object({ q: z.string().trim().max(100).optional() });
const idParamsSchema = z.object({ id: z.coerce.number().int().positive() });

type Failure = { result: "not_found" } | { result: "duplicate" } | { result: "in_use"; count: number } | { result: "invalid_tags" };

const MESSAGES = {
  university: {
    not_found: "大学が見つかりません",
    duplicate: "同じ名前の大学がすでにあります",
    in_use: (count: number) => `この大学の学部が志望校に${count}件使われているため削除できません`,
  },
  faculty: {
    not_found: "学部（または大学）が見つかりません",
    duplicate: "この大学に同じ名前の学部がすでにあります",
    in_use: (count: number) => `この学部が志望校に${count}件使われているため削除できません`,
  },
  textbookMaster: {
    not_found: "参考書が見つかりません",
    duplicate: "同じ ISBN の参考書がすでにあります",
    in_use: (count: number) => `この参考書は利用者の${count}冊に使われているため削除できません`,
  },
} as const;

function sendFailure(reply: FastifyReply, target: keyof typeof MESSAGES, outcome: Failure) {
  const messages = MESSAGES[target];
  switch (outcome.result) {
    case "not_found":
      return reply.code(404).send({ error: messages.not_found });
    case "duplicate":
      return reply.code(409).send({ error: messages.duplicate });
    case "in_use":
      return reply.code(409).send({ error: messages.in_use(outcome.count) });
    case "invalid_tags":
      return reply.code(400).send({ error: "存在しないタグが含まれています" });
  }
}

function logChange(
  request: FastifyRequest,
  adminId: string,
  change: {
    action: "create" | "update" | "delete";
    table: "University" | "Faculty" | "TextbookMaster";
    id: number;
    before?: unknown;
    after?: unknown;
  }
) {
  request.log.info({ adminId, ...change }, "admin master change");
}

export function registerAdminMasterRoutes(app: FastifyInstance) {
  app.get("/api/admin/universities", async (request, reply) => {
    if (!(await requireAdmin(request, reply))) return;
    const parsed = listQuerySchema.safeParse(request.query);
    if (!parsed.success) return reply.code(400).send({ error: parsed.error.issues });
    return listAdminUniversities({ q: parsed.data.q || undefined, page: parsed.data.page });
  });

  app.get("/api/admin/universities/:id", async (request, reply) => {
    if (!(await requireAdmin(request, reply))) return;
    const params = idParamsSchema.safeParse(request.params);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });
    const detail = await getAdminUniversityDetail(params.data.id);
    if (!detail) return reply.code(404).send({ error: MESSAGES.university.not_found });
    return detail;
  });

  app.get("/api/admin/tags", async (request, reply) => {
    if (!(await requireAdmin(request, reply))) return;
    return listAdminTags();
  });

  app.post("/api/admin/universities", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const parsed = universityInputSchema.safeParse(request.body);
    if (!parsed.success) return reply.code(400).send({ error: parsed.error.issues });

    const outcome = await createUniversity(parsed.data);
    if (outcome.result !== "ok") return sendFailure(reply, "university", outcome);
    logChange(request, session.user.id, { action: "create", table: "University", id: outcome.value.id, after: outcome.value });
    return reply.code(201).send(outcome.value);
  });

  app.patch("/api/admin/universities/:id", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const params = idParamsSchema.safeParse(request.params);
    const parsed = universityInputSchema.safeParse(request.body);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });
    if (!parsed.success) return reply.code(400).send({ error: parsed.error.issues });

    const outcome = await updateUniversity(params.data.id, parsed.data);
    if (outcome.result !== "ok") return sendFailure(reply, "university", outcome);
    logChange(request, session.user.id, { action: "update", table: "University", id: params.data.id, ...outcome.value });
    return outcome.value.after;
  });

  app.delete("/api/admin/universities/:id", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const params = idParamsSchema.safeParse(request.params);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });

    const outcome = await deleteUniversity(params.data.id);
    if (outcome.result !== "ok") return sendFailure(reply, "university", outcome);
    logChange(request, session.user.id, { action: "delete", table: "University", id: params.data.id, before: outcome.value });
    return reply.code(204).send();
  });

  app.post("/api/admin/faculties", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const parsed = createFacultySchema.safeParse(request.body);
    if (!parsed.success) return reply.code(400).send({ error: parsed.error.issues });

    const outcome = await createFaculty(parsed.data);
    if (outcome.result !== "ok") return sendFailure(reply, "faculty", outcome);
    logChange(request, session.user.id, { action: "create", table: "Faculty", id: outcome.value.id, after: outcome.value });
    return reply.code(201).send(outcome.value);
  });

  app.patch("/api/admin/faculties/:id", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const params = idParamsSchema.safeParse(request.params);
    const parsed = facultyInputSchema.safeParse(request.body);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });
    if (!parsed.success) return reply.code(400).send({ error: parsed.error.issues });

    const outcome = await updateFaculty(params.data.id, parsed.data);
    if (outcome.result !== "ok") return sendFailure(reply, "faculty", outcome);
    logChange(request, session.user.id, { action: "update", table: "Faculty", id: params.data.id, ...outcome.value });
    return outcome.value.after;
  });

  app.delete("/api/admin/faculties/:id", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const params = idParamsSchema.safeParse(request.params);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });

    const outcome = await deleteFaculty(params.data.id);
    if (outcome.result !== "ok") return sendFailure(reply, "faculty", outcome);
    logChange(request, session.user.id, { action: "delete", table: "Faculty", id: params.data.id, before: outcome.value });
    return reply.code(204).send();
  });

  app.get("/api/admin/textbook-masters", async (request, reply) => {
    if (!(await requireAdmin(request, reply))) return;
    const parsed = searchQuerySchema.safeParse(request.query);
    if (!parsed.success) return reply.code(400).send({ error: parsed.error.issues });
    return listAdminTextbookMasters(parsed.data.q || undefined);
  });

  app.post("/api/admin/textbook-masters", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const parsed = textbookMasterInputSchema.safeParse(request.body);
    if (!parsed.success) return reply.code(400).send({ error: parsed.error.issues });

    const outcome = await createTextbookMaster(parsed.data);
    if (outcome.result !== "ok") return sendFailure(reply, "textbookMaster", outcome);
    logChange(request, session.user.id, { action: "create", table: "TextbookMaster", id: outcome.value.id, after: outcome.value });
    return reply.code(201).send(outcome.value);
  });

  app.patch("/api/admin/textbook-masters/:id", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const params = idParamsSchema.safeParse(request.params);
    const parsed = textbookMasterInputSchema.safeParse(request.body);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });
    if (!parsed.success) return reply.code(400).send({ error: parsed.error.issues });

    const outcome = await updateTextbookMaster(params.data.id, parsed.data);
    if (outcome.result !== "ok") return sendFailure(reply, "textbookMaster", outcome);
    logChange(request, session.user.id, { action: "update", table: "TextbookMaster", id: params.data.id, ...outcome.value });
    return outcome.value.after;
  });

  app.delete("/api/admin/textbook-masters/:id", async (request, reply) => {
    const session = await requireAdmin(request, reply);
    if (!session) return;
    const params = idParamsSchema.safeParse(request.params);
    if (!params.success) return reply.code(400).send({ error: params.error.issues });

    const outcome = await deleteTextbookMaster(params.data.id);
    if (outcome.result !== "ok") return sendFailure(reply, "textbookMaster", outcome);
    logChange(request, session.user.id, { action: "delete", table: "TextbookMaster", id: params.data.id, before: outcome.value });
    return reply.code(204).send();
  });
}
