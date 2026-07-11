import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { createTextbookSchema } from "@/lib/validations/textbook";
import { Prisma } from "@/app/generated/prisma/client";
import { demoReadOnlyGuard } from "@/lib/demo";

export async function GET() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const textbooks = await prisma.textbook.findMany({
    where: { userId: session.user.id },
    orderBy: { name: "asc" },
  });

  return NextResponse.json(textbooks);
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
  const parsed = createTextbookSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  try {
    let textbookData: {
      name: string;
      userId: string;
      masterId?: number;
      totalAmount?: number;
      rangeUnit?: string;
    };

    if ("masterId" in parsed.data) {
      const master = await prisma.textbookMaster.findUnique({
        where: { id: parsed.data.masterId },
        include: { metrics: true },
      });
      if (!master) {
        return NextResponse.json(
          { error: "参考書マスターが見つかりません" },
          { status: 404 }
        );
      }
      const defaultMetric =
        master.metrics.find((metric) => metric.isDefault) ?? master.metrics[0];
      if (!defaultMetric) {
        return NextResponse.json(
          { error: "参考書の総量データが登録されていません" },
          { status: 400 }
        );
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
      };
    }

    const textbook = await prisma.textbook.create({
      data: textbookData,
    });

    return NextResponse.json(textbook, { status: 201 });
  } catch (error) {
    if (
      error instanceof Prisma.PrismaClientKnownRequestError &&
      error.code === "P2002"
    ) {
      return NextResponse.json(
        { error: "この参考書はすでに登録されています" },
        { status: 409 }
      );
    }
    throw error;
  }
}
