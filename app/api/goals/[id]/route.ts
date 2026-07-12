import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { updateGoalSchema, patchGoalSchema } from "@/lib/validations/goal";
import { demoReadOnlyGuard } from "@/lib/demo";

export async function PUT(
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
  const parsed = updateGoalSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const goal = await prisma.finalGoal.findUnique({
    where: { id: Number(id) },
  });

  if (!goal || goal.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const updated = await prisma.finalGoal.update({
    where: { id: Number(id) },
    data: {
      ...(parsed.data.facultyId && { facultyId: parsed.data.facultyId }),
    },
  });

  return NextResponse.json(updated);
}

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
  const parsed = patchGoalSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const goal = await prisma.finalGoal.findUnique({
    where: { id: Number(id) },
  });

  if (!goal || goal.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  // 第一志望トグル（送られてきたときだけ処理。未指定なら現状維持）
  if (parsed.data.isFirstChoice !== undefined) {
    if (parsed.data.isFirstChoice) {
      // 第一志望は1ユーザー1校まで。既存の第一志望を全部外してから付け替える
      await prisma.$transaction([
        prisma.finalGoal.updateMany({
          where: { userId: session.user.id },
          data: { isFirstChoice: false },
        }),
        prisma.finalGoal.update({
          where: { id: Number(id) },
          data: { isFirstChoice: true },
        }),
      ]);
    } else {
      await prisma.finalGoal.update({
        where: { id: Number(id) },
        data: { isFirstChoice: false },
      });
    }
  }

  // メモ更新（送られてきたときだけ）
  if (parsed.data.note !== undefined) {
    await prisma.finalGoal.update({
      where: { id: Number(id) },
      data: { note: parsed.data.note },
    });
  }

  // ステータス更新（候補→受験校に確定 など。送られてきたときだけ）
  if (parsed.data.status !== undefined) {
    await prisma.finalGoal.update({
      where: { id: Number(id) },
      data: { status: parsed.data.status },
    });
  }

  return NextResponse.json({ message: "OK" });
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

  const goal = await prisma.finalGoal.findUnique({
    where: { id: Number(id) },
  });

  if (!goal || goal.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  await prisma.finalGoal.delete({
    where: { id: Number(id) },
  });

  return NextResponse.json({ message: "Deleted" });
}
