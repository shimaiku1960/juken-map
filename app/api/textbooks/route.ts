import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { createTextbookSchema } from "@/lib/validations/textbook";
import { Prisma } from "@/app/generated/prisma/client";

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

  const body = await request.json();
  const parsed = createTextbookSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  try {
    const textbook = await prisma.textbook.create({
      data: {
        name: parsed.data.name,
        userId: session.user.id,
      },
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
