import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { demoReadOnlyGuard } from "@/lib/demo";
import { completeStudyPlanSchema } from "@/lib/validations/studyLog";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const { id } = await params;
  const planId = Number(id);
  if (!Number.isInteger(planId) || planId <= 0) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const parsed = completeStudyPlanSchema.safeParse(await request.json());
  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const plan = await prisma.studyPlan.findFirst({
    where: { id: planId, userId: session.user.id },
    include: { textbook: true, studyLog: true },
  });
  if (!plan) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }
  if (plan.studyLog) {
    return NextResponse.json(
      { error: "この予定の実績はすでに記録されています" },
      { status: 409 }
    );
  }

  const rangeStart =
    parsed.data.rangeStart !== undefined ? parsed.data.rangeStart : plan.rangeStart;
  const rangeEnd =
    parsed.data.rangeEnd !== undefined ? parsed.data.rangeEnd : plan.rangeEnd;
  const rangeUnit =
    parsed.data.rangeUnit !== undefined ? parsed.data.rangeUnit : plan.rangeUnit;
  if (
    rangeEnd != null &&
    plan.textbook?.rangeUnit != null &&
    rangeUnit !== plan.textbook.rangeUnit
  ) {
    return NextResponse.json(
      { error: "範囲の単位を参考書の逆算設定に合わせてください" },
      { status: 400 }
    );
  }
  if (
    rangeEnd != null &&
    plan.textbook?.totalAmount != null &&
    rangeEnd > plan.textbook.totalAmount
  ) {
    return NextResponse.json(
      { error: `終了位置は参考書の総量（${plan.textbook.totalAmount}）以下にしてください` },
      { status: 400 }
    );
  }

  const { log, updatedPlan, isFirstStudyLog } = await prisma.$transaction(
    async (tx) => {
      const activation = await tx.user.updateMany({
        where: { id: session.user.id, firstStudyLogAt: null },
        data: { firstStudyLogAt: new Date() },
      });
      const log = await tx.studyLog.create({
      data: {
        userId: session.user.id,
        studyPlanId: plan.id,
        date: plan.date,
        minutes: parsed.data.minutes,
        subject: plan.subject,
        textbookId: plan.textbookId,
        rangeStart,
        rangeEnd,
        rangeUnit,
        memo: parsed.data.memo ?? null,
      },
      include: { textbook: true },
      });
      const updatedPlan = await tx.studyPlan.update({
        where: { id: plan.id },
        data: { done: true },
      });
      return {
        log,
        updatedPlan,
        isFirstStudyLog: activation.count === 1,
      };
    }
  );

  return NextResponse.json(
    { log, plan: updatedPlan, isFirstStudyLog },
    { status: 201 }
  );
}
