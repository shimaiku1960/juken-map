import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { createStudyPlansSchema } from "@/lib/validations/studyPlan";
import { demoReadOnlyGuard } from "@/lib/demo";

export async function GET() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const plans = await prisma.studyPlan.findMany({
    where: { userId: session.user.id },
    orderBy: { date: "asc" },
    include: { textbook: true },
  });

  return NextResponse.json(plans);
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
  const parsed = createStudyPlansSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const date = new Date(parsed.data.date);

  // 参考書は他人のIDを混ぜられないよう、自分の所有分だけを許可する
  const textbookIds = [
    ...new Set(
      parsed.data.items
        .map((item) => item.textbookId)
        .filter((id): id is number => id != null)
    ),
  ];

  if (textbookIds.length > 0) {
    const owned = await prisma.textbook.count({
      where: { id: { in: textbookIds }, userId: session.user.id },
    });
    if (owned !== textbookIds.length) {
      return NextResponse.json(
        { error: "不正な参考書が含まれています" },
        { status: 400 }
      );
    }
  }

  // 1つの日付に複数の内容をまとめて作成
  const result = await prisma.studyPlan.createMany({
    data: parsed.data.items.map((item) => ({
      date,
      textbookId: item.textbookId ?? null,
      rangeStart: item.rangeStart ?? null,
      rangeEnd: item.rangeEnd ?? null,
      rangeUnit: item.rangeUnit ?? null,
      content: item.content ?? null,
      subject: item.subject ?? null,
      userId: session.user.id,
    })),
  });

  return NextResponse.json({ count: result.count }, { status: 201 });
}
