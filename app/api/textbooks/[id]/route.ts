import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { demoReadOnlyGuard } from "@/lib/demo";
import { updateTextbookProgressSchema } from "@/lib/validations/textbook";

export async function PATCH(
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
  const textbookId = Number(id);
  if (!Number.isInteger(textbookId)) {
    return NextResponse.json({ error: "Invalid textbook id" }, { status: 400 });
  }

  const parsed = updateTextbookProgressSchema.safeParse(await request.json());
  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const textbook = await prisma.textbook.findFirst({
    where: { id: textbookId, userId: session.user.id },
  });
  if (!textbook) {
    return NextResponse.json({ error: "参考書が見つかりません" }, { status: 404 });
  }

  const updated = await prisma.textbook.update({
    where: { id: textbookId },
    data: {
      totalAmount: parsed.data.totalAmount,
      rangeUnit: parsed.data.rangeUnit,
      targetDate:
        parsed.data.targetDate == null
          ? null
          : new Date(`${parsed.data.targetDate}T00:00:00.000Z`),
      // subject は送られてきたときだけ更新（未指定なら現状維持）
      ...(parsed.data.subject !== undefined && { subject: parsed.data.subject }),
    },
  });

  return NextResponse.json(updated);
}
