import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { demoReadOnlyGuard } from "@/lib/demo";
import prisma from "@/lib/prisma";

export async function DELETE() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) return NextResponse.json({ error: "未認証" }, { status: 401 });
  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  await prisma.$transaction([
    prisma.notificationPreference.updateMany({
      where: { userId: session.user.id },
      data: { lineMorningEnabled: false, lineEveningEnabled: false },
    }),
    prisma.lineConnection.deleteMany({ where: { userId: session.user.id } }),
    prisma.lineLinkNonce.deleteMany({ where: { userId: session.user.id } }),
    prisma.lineOAuthAttempt.deleteMany({ where: { userId: session.user.id } }),
  ]);
  return NextResponse.json({ connected: false });
}
