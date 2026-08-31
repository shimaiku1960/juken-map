import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { demoReadOnlyGuard } from "@/lib/demo";
import prisma from "@/lib/prisma";
import { notificationPreferenceSchema } from "@/lib/validations/notification";

const DEFAULT_PREFERENCE = {
  emailMorningEnabled: false,
  emailEveningEnabled: false,
  lineMorningEnabled: false,
  lineEveningEnabled: false,
};

export async function GET() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "未認証" }, { status: 401 });
  }

  const preference = await prisma.notificationPreference.findUnique({
    where: { userId: session.user.id },
    select: { morningEnabled: true, eveningEnabled: true, lineMorningEnabled: true, lineEveningEnabled: true },
  });

  return NextResponse.json(preference ? {
    emailMorningEnabled: preference.morningEnabled,
    emailEveningEnabled: preference.eveningEnabled,
    lineMorningEnabled: preference.lineMorningEnabled,
    lineEveningEnabled: preference.lineEveningEnabled,
  } : DEFAULT_PREFERENCE);
}

export async function PUT(request: Request) {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "未認証" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const result = notificationPreferenceSchema.safeParse(await request.json());
  if (!result.success) {
    return NextResponse.json(
      { error: result.error.issues[0].message },
      { status: 400 }
    );
  }

  if (result.data.lineMorningEnabled || result.data.lineEveningEnabled) {
    const connection = await prisma.lineConnection.findUnique({ where: { userId: session.user.id }, select: { id: true } });
    if (!connection) return NextResponse.json({ error: "LINEと連携してからLINE通知を選択してください" }, { status: 400 });
  }

  const data = {
    morningEnabled: result.data.emailMorningEnabled,
    eveningEnabled: result.data.emailEveningEnabled,
    lineMorningEnabled: result.data.lineMorningEnabled,
    lineEveningEnabled: result.data.lineEveningEnabled,
  };

  const preference = await prisma.notificationPreference.upsert({
    where: { userId: session.user.id },
    create: { userId: session.user.id, ...data },
    update: data,
    select: { morningEnabled: true, eveningEnabled: true, lineMorningEnabled: true, lineEveningEnabled: true },
  });

  return NextResponse.json({
    emailMorningEnabled: preference.morningEnabled,
    emailEveningEnabled: preference.eveningEnabled,
    lineMorningEnabled: preference.lineMorningEnabled,
    lineEveningEnabled: preference.lineEveningEnabled,
  });
}
