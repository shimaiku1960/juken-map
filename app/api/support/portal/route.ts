import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { demoReadOnlyGuard } from "@/lib/demo";
import prisma from "@/lib/prisma";
import { SITE_URL } from "@/lib/site";
import { getStripe } from "@/lib/stripe";

export const runtime = "nodejs";

function appUrl() {
  return process.env.BETTER_AUTH_URL || SITE_URL;
}

export async function POST() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "ログインが必要です" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const subscription = await prisma.supportSubscription.findUnique({
    where: { userId: session.user.id },
    select: { stripeCustomerId: true },
  });
  if (!subscription) {
    return NextResponse.json(
      { error: "管理できるサポート契約がありません" },
      { status: 404 }
    );
  }

  const portalSession = await getStripe().billingPortal.sessions.create({
    customer: subscription.stripeCustomerId,
    return_url: `${appUrl()}/dashboard`,
    locale: "ja",
  });

  return NextResponse.json({ url: portalSession.url });
}
