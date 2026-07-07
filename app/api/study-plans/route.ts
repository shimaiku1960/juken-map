import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { createStudyPlansSchema } from "@/lib/validations/studyPlan";

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

  const body = await request.json();
  const parsed = createStudyPlansSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const date = new Date(parsed.data.date);

  // 1つの日付に複数の内容をまとめて作成
  const result = await prisma.studyPlan.createMany({
    data: parsed.data.items.map((item) => ({
      date,
      content: item.content,
      subject: item.subject ?? null,
      userId: session.user.id,
    })),
  });

  return NextResponse.json({ count: result.count }, { status: 201 });
}
