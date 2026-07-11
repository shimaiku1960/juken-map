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

  // 参考書を指定する場合は、自分の所有分だけを許可する（他人のIDを弾く）
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

  const created = await prisma.studyLog.create({
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

  return NextResponse.json(created, { status: 201 });
}
