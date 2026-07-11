import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { demoReadOnlyGuard } from "@/lib/demo";

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
