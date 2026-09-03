import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { createStudyLogSchema } from "@/lib/validations/studyLog";
import { demoReadOnlyGuard } from "@/lib/demo";

export async function GET() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const logs = await prisma.studyLog.findMany({
    where: { userId: session.user.id },
    orderBy: { date: "desc" },
    include: { textbook: true },
  });

  return NextResponse.json(logs);
}

export async function POST(request: Request) {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const body = await request.json();
  const parsed = createStudyLogSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  // 参考書を指定する場合は、所有権と逆算設定との整合性を検証する。
  if (parsed.data.textbookId != null) {
    const textbook = await prisma.textbook.findFirst({
      where: { id: parsed.data.textbookId, userId: session.user.id },
    });
    if (!textbook) {
      return NextResponse.json(
        { error: "不正な参考書です" },
        { status: 400 }
      );
    }

    if (
      parsed.data.rangeEnd != null &&
      textbook.rangeUnit != null &&
      parsed.data.rangeUnit !== textbook.rangeUnit
    ) {
      return NextResponse.json(
        { error: "範囲の単位を参考書の逆算設定に合わせてください" },
        { status: 400 }
      );
    }

    if (
      parsed.data.rangeEnd != null &&
      textbook.totalAmount != null &&
      parsed.data.rangeEnd > textbook.totalAmount
    ) {
      return NextResponse.json(
        { error: `終了位置は参考書の総量（${textbook.totalAmount}）以下にしてください` },
        { status: 400 }
      );
    }
  }

  const { created, isFirstStudyLog } = await prisma.$transaction(async (tx) => {
    const activation = await tx.user.updateMany({
      where: { id: session.user.id, firstStudyLogAt: null },
      data: { firstStudyLogAt: new Date() },
    });
    const created = await tx.studyLog.create({
      data: {
        date: new Date(parsed.data.date),
        minutes: parsed.data.minutes,
        subject: parsed.data.subject ?? null,
        textbookId: parsed.data.textbookId ?? null,
        rangeStart: parsed.data.rangeStart ?? null,
        rangeEnd: parsed.data.rangeEnd ?? null,
        rangeUnit: parsed.data.rangeUnit ?? null,
        memo: parsed.data.memo ?? null,
        userId: session.user.id,
      },
    });
    return { created, isFirstStudyLog: activation.count === 1 };
  });

  return NextResponse.json(
    { ...created, isFirstStudyLog },
    { status: 201 }
  );
}
