import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { demoReadOnlyGuard } from "@/lib/demo";
import prisma from "@/lib/prisma";
import {
  createSupportLineCode,
  hasSupportLineAccess,
  hashSupportLineCode,
  supportLineCodeExpiresAt,
} from "@/lib/support-line";

export const runtime = "nodejs";

export async function POST() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "ログインが必要です" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const [subscription, connection] = await Promise.all([
    prisma.supportSubscription.findUnique({
      where: { userId: session.user.id },
      select: { status: true },
    }),
    prisma.supportLineConnection.findUnique({
      where: { userId: session.user.id },
      select: { linkedAt: true },
    }),
  ]);

  if (!subscription || !hasSupportLineAccess(subscription.status)) {
    return NextResponse.json(
      { error: "有効なサポート契約が必要です" },
      { status: 403 }
    );
  }

  if (connection?.linkedAt) {
    return NextResponse.json(
      { error: "LINEアカウントは連携済みです" },
      { status: 409 }
    );
  }

  const code = createSupportLineCode();
  const codeHash = hashSupportLineCode(code);
  const expiresAt = supportLineCodeExpiresAt();
  await prisma.supportLineConnection.upsert({
    where: { userId: session.user.id },
    create: {
      userId: session.user.id,
      codeHash,
      codeExpiresAt: expiresAt,
    },
    update: {
      codeHash,
      codeExpiresAt: expiresAt,
      lineDisplayName: null,
    },
  });

  return NextResponse.json({ code, expiresAt: expiresAt.toISOString() });
}
