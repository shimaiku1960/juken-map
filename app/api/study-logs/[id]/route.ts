import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { demoReadOnlyGuard } from "@/lib/demo";
import { createStudyLogSchema } from "@/lib/validations/studyLog";

async function findOwnedTextbook(
  textbookId: number,
  userId: string
) {
  return prisma.textbook.findFirst({
    where: { id: textbookId, userId },
  });
}

function textbookValidationError(
  textbook: {
    rangeUnit: string | null;
    totalAmount: number | null;
  },
  data: {
    rangeEnd?: number | null;
    rangeUnit?: string | null;
  }
) {
  if (
    data.rangeEnd != null &&
    textbook.rangeUnit != null &&
    data.rangeUnit !== textbook.rangeUnit
  ) {
    return "範囲の単位を参考書の逆算設定に合わせてください";
  }
  if (
    data.rangeEnd != null &&
    textbook.totalAmount != null &&
    data.rangeEnd > textbook.totalAmount
  ) {
    return `終了位置は参考書の総量（${textbook.totalAmount}）以下にしてください`;
  }
  return null;
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
  const logId = Number(id);
  const log = await prisma.studyLog.findUnique({ where: { id: logId } });

  if (!log || log.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const parsed = createStudyLogSchema.safeParse(await request.json());
  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const rangeChanged =
    parsed.data.rangeStart !== log.rangeStart ||
    parsed.data.rangeEnd !== log.rangeEnd ||
    parsed.data.rangeUnit !== log.rangeUnit;
  const textbookChanged =
    log.studyPlanId == null && parsed.data.textbookId !== log.textbookId;
  const effectiveTextbookId =
    log.studyPlanId != null ? log.textbookId : parsed.data.textbookId;

  // 時間・メモだけの修正では、後から変わった参考書設定を過去実績へ遡及しない。
  if (effectiveTextbookId != null && (rangeChanged || textbookChanged)) {
    const textbook = await findOwnedTextbook(
      effectiveTextbookId,
      session.user.id
    );
    if (!textbook) {
      return NextResponse.json({ error: "不正な参考書です" }, { status: 400 });
    }
    const validationError = textbookValidationError(textbook, parsed.data);
    if (validationError) {
      return NextResponse.json({ error: validationError }, { status: 400 });
    }
  }

  const updated = await prisma.studyLog.update({
    where: { id: logId },
    data: {
      // 予定から作成した実績は、予定との紐づきを壊す項目を固定する。
      date:
        log.studyPlanId != null ? log.date : new Date(parsed.data.date),
      minutes: parsed.data.minutes,
      subject:
        log.studyPlanId != null ? log.subject : parsed.data.subject ?? null,
      textbookId:
        log.studyPlanId != null
          ? log.textbookId
          : parsed.data.textbookId ?? null,
      rangeStart: parsed.data.rangeStart ?? null,
      rangeEnd: parsed.data.rangeEnd ?? null,
      rangeUnit: parsed.data.rangeUnit ?? null,
      memo: parsed.data.memo ?? null,
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

  const log = await prisma.studyLog.findUnique({
    where: { id: Number(id) },
  });

  if (!log || log.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  await prisma.studyLog.delete({
    where: { id: Number(id) },
  });

  return NextResponse.json({ message: "Deleted" });
}
