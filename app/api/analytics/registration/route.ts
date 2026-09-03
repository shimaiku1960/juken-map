import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

export async function POST() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const tracked = await prisma.user.updateMany({
    where: { id: session.user.id, analyticsSignUpTrackedAt: null },
    data: { analyticsSignUpTrackedAt: new Date() },
  });
  if (tracked.count === 0) {
    return NextResponse.json({ shouldTrack: false });
  }

  const account = await prisma.account.findFirst({
    where: { userId: session.user.id },
    select: { providerId: true },
    orderBy: { createdAt: "asc" },
  });
  const method =
    account?.providerId === "google" || account?.providerId === "github"
      ? account.providerId
      : "email";

  return NextResponse.json({ shouldTrack: true, method });
}
