import { createHash } from "node:crypto";
import type Stripe from "stripe";
import { Prisma } from "@/app/generated/prisma/client";
import prisma from "@/lib/prisma";
import { getStripe } from "@/lib/stripe";

export const ACTIVE_SUPPORT_SUBSCRIPTION_STATUSES = [
  "trialing",
  "active",
  "past_due",
] as const;

export function hashSupportInvitationToken(token: string) {
  return createHash("sha256").update(token).digest("hex");
}

function unixSecondsToDate(value: number | null | undefined) {
  return value == null ? null : new Date(value * 1000);
}

export async function syncSupportSubscription(
  subscription: Stripe.Subscription,
  fallbackUserId?: string
) {
  const userId = subscription.metadata.userId || fallbackUserId;
  if (!userId) {
    throw new Error(`Stripe subscription ${subscription.id} has no userId metadata`);
  }

  const customerId =
    typeof subscription.customer === "string"
      ? subscription.customer
      : subscription.customer.id;
  const item = subscription.items.data[0];

  const subscriptionData = {
    stripeCustomerId: customerId,
    stripeSubscriptionId: subscription.id,
    status: subscription.status,
    trialUsed: subscription.trial_start != null,
    trialEndsAt: unixSecondsToDate(subscription.trial_end),
    currentPeriodEndsAt: unixSecondsToDate(item?.current_period_end),
    cancelAtPeriodEnd: subscription.cancel_at_period_end,
    canceledAt: unixSecondsToDate(subscription.canceled_at),
  };

  try {
    return await prisma.supportSubscription.upsert({
      where: { userId },
      create: { userId, ...subscriptionData },
      update: subscriptionData,
    });
  } catch (error) {
    // Checkout完了画面とWebhookが同時に初回作成すると、MySQL adapterでは
    // upsert同士が競合して片方だけP2002になることがある。先行作成行へ再同期する。
    if (
      error instanceof Prisma.PrismaClientKnownRequestError &&
      error.code === "P2002"
    ) {
      return prisma.supportSubscription.update({
        where: { userId },
        data: subscriptionData,
      });
    }
    throw error;
  }
}

export async function reconcileSupportCheckoutSession(
  checkoutSessionId: string,
  expectedUserId?: string
) {
  const checkoutSession = await getStripe().checkout.sessions.retrieve(
    checkoutSessionId
  );
  const userId = checkoutSession.metadata?.userId;
  const invitationId = checkoutSession.metadata?.invitationId;

  if (
    checkoutSession.status !== "complete" ||
    checkoutSession.mode !== "subscription" ||
    !checkoutSession.subscription ||
    !userId ||
    !invitationId ||
    (expectedUserId && userId !== expectedUserId)
  ) {
    throw new Error(`Checkout Session ${checkoutSessionId} is not reconcilable`);
  }

  const subscriptionId =
    typeof checkoutSession.subscription === "string"
      ? checkoutSession.subscription
      : checkoutSession.subscription.id;
  const subscription = await getStripe().subscriptions.retrieve(subscriptionId);

  await syncSupportSubscription(subscription, userId);
  await prisma.supportCheckoutInvitation.update({
    where: { id: invitationId },
    data: {
      userId,
      checkoutSessionId: checkoutSession.id,
      usedAt: new Date(),
    },
  });

  return subscription;
}
