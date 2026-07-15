import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { updateStudyPlanSchema } from "@/lib/validations/studyPlan";
import { demoReadOnlyGuard } from "@/lib/demo";

export async function PATCH(
  request: Request,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const { id } = await params;

  const body = await request.json();
  const parsed = updateStudyPlanSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const plan = await prisma.studyPlan.findUnique({
    where: { id: Number(id) },
  });

  if (!plan || plan.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  if (parsed.data.done === false) {
    const linkedLog = await prisma.studyLog.count({
      where: { studyPlanId: plan.id },
    });
    if (linkedLog > 0) {
      return NextResponse.json(
        { error: "実績を記録済みの予定は未完了に戻せません" },
        { status: 409 }
      );
    }
  }

  // 参考書を指定する場合は、自分の所有分だけを許可する
  if (parsed.data.textbookId != null) {
    const owned = await prisma.textbook.count({
      where: { id: parsed.data.textbookId, userId: session.user.id },
    });
    if (owned === 0) {
      return NextResponse.json(
        { error: "不正な参考書です" },
        { status: 400 }
      );
    }
  }

  const updated = await prisma.studyPlan.update({
    where: { id: Number(id) },
    data: {
      ...(parsed.data.date && { date: new Date(parsed.data.date) }),
      ...(parsed.data.content !== undefined && {
        content: parsed.data.content,
      }),
      ...(parsed.data.subject !== undefined && {
        subject: parsed.data.subject,
      }),
      ...(parsed.data.textbookId !== undefined && {
        textbookId: parsed.data.textbookId,
      }),
      ...(parsed.data.rangeStart !== undefined && {
        rangeStart: parsed.data.rangeStart,
      }),
      ...(parsed.data.rangeEnd !== undefined && {
        rangeEnd: parsed.data.rangeEnd,
      }),
      ...(parsed.data.rangeUnit !== undefined && {
        rangeUnit: parsed.data.rangeUnit,
      }),
      ...(parsed.data.done !== undefined && { done: parsed.data.done }),
    },
  });

  return NextResponse.json(updated);
}

export async function DELETE(
  _request: Request,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const { id } = await params;

  const plan = await prisma.studyPlan.findUnique({
    where: { id: Number(id) },
  });

  if (!plan || plan.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  await prisma.studyPlan.delete({
    where: { id: Number(id) },
  });

  return NextResponse.json({ message: "Deleted" });
}
