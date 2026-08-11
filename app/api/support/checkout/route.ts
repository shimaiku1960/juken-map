import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { demoReadOnlyGuard } from "@/lib/demo";
import prisma from "@/lib/prisma";
import { SITE_URL } from "@/lib/site";
import { getStripe, getSupportPriceId } from "@/lib/stripe";
import { SUPPORT_TRIAL_DAYS } from "@/lib/support";
import { hashSupportInvitationToken } from "@/lib/support-subscription";

export const runtime = "nodejs";

const checkoutSchema = z.object({
  invitationToken: z.string().min(32).max(512),
});

function appUrl() {
  return process.env.BETTER_AUTH_URL || SITE_URL;
}

export async function POST(request: Request) {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "ログインが必要です" }, { status: 401 });
  }

  const denied = demoReadOnlyGuard(session);
  if (denied) return denied;

  const parsed = checkoutSchema.safeParse(await request.json());
  if (!parsed.success) {
    return NextResponse.json({ error: "招待URLが正しくありません" }, { status: 400 });
  }

  const invitation = await prisma.supportCheckoutInvitation.findUnique({
    where: { tokenHash: hashSupportInvitationToken(parsed.data.invitationToken) },
  });
  const normalizedSessionEmail = session.user.email?.trim().toLowerCase();

  if (
    !invitation ||
    invitation.usedAt ||
    invitation.expiresAt <= new Date() ||
    invitation.email.trim().toLowerCase() !== normalizedSessionEmail
  ) {
    return NextResponse.json(
      { error: "この招待URLは無効、期限切れ、または別のメールアドレス用です" },
      { status: 403 }
    );
  }

  const existingSubscription = await prisma.supportSubscription.findUnique({
    where: { userId: session.user.id },
  });
  if (existingSubscription) {
    return NextResponse.json(
      { error: "このアカウントでは無料体験または契約をすでに開始しています" },
      { status: 409 }
    );
  }

  const stripe = getStripe();
  const customer = await stripe.customers.create(
    {
      email: normalizedSessionEmail,
      metadata: { userId: session.user.id },
    },
    { idempotencyKey: `support-customer-${invitation.id}` }
  );

  const checkoutSession = await stripe.checkout.sessions.create(
    {
      mode: "subscription",
      customer: customer.id,
      line_items: [{ price: getSupportPriceId(), quantity: 1 }],
      payment_method_collection: "always",
      subscription_data: {
        trial_period_days: SUPPORT_TRIAL_DAYS,
        metadata: {
          userId: session.user.id,
          invitationId: invitation.id,
        },
      },
      metadata: {
        userId: session.user.id,
        invitationId: invitation.id,
      },
      success_url: `${appUrl()}/support/apply/success?session_id={CHECKOUT_SESSION_ID}`,
      cancel_url: `${appUrl()}/support/apply?invitation=${encodeURIComponent(parsed.data.invitationToken)}`,
    },
    { idempotencyKey: `support-checkout-${invitation.id}` }
  );

  if (!checkoutSession.url) {
    return NextResponse.json({ error: "決済画面を開始できませんでした" }, { status: 502 });
  }

  await prisma.supportCheckoutInvitation.update({
    where: { id: invitation.id },
    data: {
      userId: session.user.id,
      checkoutSessionId: checkoutSession.id,
    },
  });

  return NextResponse.json({ url: checkoutSession.url });
}
