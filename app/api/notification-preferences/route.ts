import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { demoReadOnlyGuard } from "@/lib/demo";
import prisma from "@/lib/prisma";
import { notificationPreferenceSchema } from "@/lib/validations/notification";

const DEFAULT_PREFERENCE = {
  morningEnabled: false,
  eveningEnabled: false,
};

export async function GET() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "未認証" }, { status: 401 });
  }

  const preference = await prisma.notificationPreference.findUnique({
    where: { userId: session.user.id },
    select: { morningEnabled: true, eveningEnabled: true },
  });

  return NextResponse.json(preference ?? DEFAULT_PREFERENCE);
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

  const preference = await prisma.notificationPreference.upsert({
    where: { userId: session.user.id },
    create: { userId: session.user.id, ...result.data },
    update: result.data,
    select: { morningEnabled: true, eveningEnabled: true },
  });

  return NextResponse.json(preference);
}
