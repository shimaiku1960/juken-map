import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { NextResponse } from "next/server";
import { profileSchema } from "@/lib/validations/profile";
import { demoReadOnlyGuard } from "@/lib/demo";

export const PUT = async (request: Request) => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    return NextResponse.json({ error: "未認証" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const body = await request.json();
  const result = profileSchema.safeParse(body);

  if (!result.success) {
    return NextResponse.json(
      { error: result.error.issues[0].message },
      { status: 400 }
    );
  }

  const user = await prisma.user.update({
    where: { id: session.user.id },
    data: { nickname: result.data.nickname },
  });

  return NextResponse.json(user);
};
