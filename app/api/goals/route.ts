import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma  from "@/lib/prisma";
import { goalSchema } from "@/lib/validations/goal";
import { Prisma } from "@/app/generated/prisma/client";
import { demoReadOnlyGuard } from "@/lib/demo";




export async function GET() {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const goals = await prisma.finalGoal.findMany({
    where: { userId: session.user.id },
    orderBy: { createdAt: "asc" },
    include: {
      faculty: {
        include: { university: true, tags: true },
      },
    },
  });

  return NextResponse.json(goals);
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
  const parsed = goalSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  try {
    const goal = await prisma.finalGoal.create({
      data: {
        facultyId: parsed.data.facultyId,
        userId: session.user.id,
        status: parsed.data.status ?? "decided",
      },
      include: {
        faculty: {
          include: { university: true },
        },
      },
    });

    return NextResponse.json(goal, { status: 201 });
  } catch (error) {
    if (
      error instanceof Prisma.PrismaClientKnownRequestError &&
      error.code === "P2002"
    ) {
      return NextResponse.json(
        { error: "この学部はすでに登録されています" },
        { status: 409 }
      );
    }
    throw error;
  }
}
