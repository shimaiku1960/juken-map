import type { FastifyInstance } from "fastify";
import { prisma } from "@/backend/infra/prisma";
import { Prisma } from "@/app/generated/prisma/client";
import {
  createTextbookSchema,
  updateTextbookProgressSchema,
} from "@/shared/validations/textbook";
import { denyDemoWrite, requireSession } from "../context.ts";

type IdParams = { id: string };

export function registerTextbookRoutes(app: FastifyInstance) {
  app.get("/api/textbooks", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    return prisma.textbook.findMany({
      where: { userId: session.user.id },
      orderBy: { name: "asc" },
    });
  });

  app.post("/api/textbooks", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const parsed = createTextbookSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    try {
      let textbookData: {
        name: string;
        userId: string;
        masterId?: number;
        totalAmount?: number;
        rangeUnit?: string;
        subject?: string | null;
      };

      if ("masterId" in parsed.data) {
        const master = await prisma.textbookMaster.findUnique({
          where: { id: parsed.data.masterId },
          include: { metrics: true },
        });
        if (!master) {
          return reply.code(404).send({ error: "参考書マスターが見つかりません" });
        }
        const defaultMetric =
          master.metrics.find((metric) => metric.isDefault) ?? master.metrics[0];
        if (!defaultMetric) {
          return reply
            .code(400)
            .send({ error: "参考書の総量データが登録されていません" });
        }
        textbookData = {
          name: master.name,
          userId: session.user.id,
          masterId: master.id,
          totalAmount: defaultMetric.totalAmount,
          rangeUnit: defaultMetric.unit,
        };
      } else {
        textbookData = {
          name: parsed.data.name,
          userId: session.user.id,
          subject: parsed.data.subject,
          rangeUnit: parsed.data.rangeUnit,
        };
      }

      const textbook = await prisma.textbook.create({ data: textbookData });
      return reply.code(201).send(textbook);
    } catch (error) {
      if (
        error instanceof Prisma.PrismaClientKnownRequestError &&
        error.code === "P2002"
      ) {
        return reply.code(409).send({ error: "この参考書はすでに登録されています" });
      }
      throw error;
    }
  });

  app.patch<{ Params: IdParams }>("/api/textbooks/:id", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    const textbookId = Number(request.params.id);
    if (!Number.isInteger(textbookId)) {
      return reply.code(400).send({ error: "Invalid textbook id" });
    }

    const parsed = updateTextbookProgressSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    const textbook = await prisma.textbook.findFirst({
      where: { id: textbookId, userId: session.user.id },
    });
    if (!textbook) {
      return reply.code(404).send({ error: "参考書が見つかりません" });
    }

    return prisma.textbook.update({
      where: { id: textbookId },
      data: {
        // 送られてきた項目だけ更新（未指定なら現状維持）
        ...(parsed.data.totalAmount !== undefined && {
          totalAmount: parsed.data.totalAmount,
        }),
        ...(parsed.data.rangeUnit !== undefined && {
          rangeUnit: parsed.data.rangeUnit,
        }),
        ...(parsed.data.targetDate !== undefined && {
          targetDate:
            parsed.data.targetDate == null
              ? null
              : new Date(`${parsed.data.targetDate}T00:00:00.000Z`),
        }),
        ...(parsed.data.subject !== undefined && { subject: parsed.data.subject }),
      },
    });
  });
}
