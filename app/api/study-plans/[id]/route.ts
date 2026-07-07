import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { updateStudyPlanSchema } from "@/lib/validations/studyPlan";

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

  const { id } = await params;

  const body = await request.json();
  const parsed = updateStudyPlanSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.issues }, { status: 400 });
  }

  const plan = await prisma.studyPlan.findUnique({
    where: { id: Number(id) },
  });

  if (!plan || plan.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const updated = await prisma.studyPlan.update({
    where: { id: Number(id) },
    data: {
      ...(parsed.data.date && { date: new Date(parsed.data.date) }),
      ...(parsed.data.content !== undefined && {
        content: parsed.data.content,
      }),
      ...(parsed.data.subject !== undefined && {
        subject: parsed.data.subject,
      }),
      ...(parsed.data.done !== undefined && { done: parsed.data.done }),
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

  const { id } = await params;

  const plan = await prisma.studyPlan.findUnique({
    where: { id: Number(id) },
  });

  if (!plan || plan.userId !== session.user.id) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  await prisma.studyPlan.delete({
    where: { id: Number(id) },
  });

  return NextResponse.json({ message: "Deleted" });
}
