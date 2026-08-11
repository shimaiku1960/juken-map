import { describe, expect, it, vi } from "vitest";
import type Stripe from "stripe";
import { mapSupportSubscription } from "@/lib/support-subscription";

vi.mock("@/lib/prisma", () => ({ default: {} }));

describe("mapSupportSubscription", () => {
  it("無料体験終了日の予定解約をcancelAtとして変換する", () => {
    const cancelAt = 1_786_982_400;
    const subscription = {
      id: "sub_test",
      customer: "cus_test",
      status: "trialing",
      trial_start: 1_786_377_600,
      trial_end: cancelAt,
      cancel_at_period_end: false,
      cancel_at: cancelAt,
      canceled_at: 1_786_377_700,
      items: { data: [{ current_period_end: cancelAt }] },
    } as unknown as Stripe.Subscription;

    expect(mapSupportSubscription(subscription)).toEqual({
      stripeCustomerId: "cus_test",
      stripeSubscriptionId: "sub_test",
      status: "trialing",
      trialUsed: true,
      trialEndsAt: new Date(cancelAt * 1000),
      currentPeriodEndsAt: new Date(cancelAt * 1000),
      cancelAtPeriodEnd: false,
      cancelAt: new Date(cancelAt * 1000),
      canceledAt: new Date(1_786_377_700 * 1000),
    });
  });
});
