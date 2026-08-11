import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { POST } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { getStripe } from "@/lib/stripe";

vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    supportCheckoutInvitation: {
      findUnique: vi.fn(),
      update: vi.fn(),
    },
    supportSubscription: { findUnique: vi.fn() },
  },
}));

vi.mock("@/lib/stripe", () => ({
  getStripe: vi.fn(),
  getSupportPriceId: vi.fn(() => "price_test_support"),
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findInvitation = prisma.supportCheckoutInvitation.findUnique as unknown as Mock;
const updateInvitation = prisma.supportCheckoutInvitation.update as unknown as Mock;
const findSubscription = prisma.supportSubscription.findUnique as unknown as Mock;
const createCustomer = vi.fn();
const createCheckoutSession = vi.fn();
const invitationToken = "a".repeat(32);

const request = () =>
  ({ json: async () => ({ invitationToken }) } as unknown as Request);

const validInvitation = {
  id: "invite-1",
  email: "student@example.com",
  expiresAt: new Date(Date.now() + 60_000),
  usedAt: null,
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getStripe).mockReturnValue({
    customers: { create: createCustomer },
    checkout: { sessions: { create: createCheckoutSession } },
  } as unknown as ReturnType<typeof getStripe>);
  createCustomer.mockResolvedValue({ id: "cus_test" });
  createCheckoutSession.mockResolvedValue({
    id: "cs_test",
    url: "https://checkout.stripe.test/session",
  });
  updateInvitation.mockResolvedValue(validInvitation);
  findSubscription.mockResolvedValue(null);
});

describe("POST /api/support/checkout", () => {
  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    const response = await POST(request());

    expect(response.status).toBe(401);
    expect(createCheckoutSession).not.toHaveBeenCalled();
  });

  it("招待先とログインメールが異なる場合は403を返す", async () => {
    getSession.mockResolvedValue({
      user: { id: "user-1", email: "other@example.com" },
    });
    findInvitation.mockResolvedValue(validInvitation);

    const response = await POST(request());

    expect(response.status).toBe(403);
    expect(createCheckoutSession).not.toHaveBeenCalled();
  });

  it("無料体験または契約の履歴がある場合は409を返す", async () => {
    getSession.mockResolvedValue({
      user: { id: "user-1", email: "student@example.com" },
    });
    findInvitation.mockResolvedValue(validInvitation);
    findSubscription.mockResolvedValue({ id: "subscription-1" });

    const response = await POST(request());

    expect(response.status).toBe(409);
    expect(createCheckoutSession).not.toHaveBeenCalled();
  });

  it("7日間無料のサブスクリプションCheckoutを作成する", async () => {
    getSession.mockResolvedValue({
      user: { id: "user-1", email: "student@example.com" },
    });
    findInvitation.mockResolvedValue(validInvitation);

    const response = await POST(request());

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      url: "https://checkout.stripe.test/session",
    });
    expect(createCheckoutSession).toHaveBeenCalledWith(
      expect.objectContaining({
        mode: "subscription",
        customer: "cus_test",
        line_items: [{ price: "price_test_support", quantity: 1 }],
        payment_method_collection: "always",
        subscription_data: expect.objectContaining({
          trial_period_days: 7,
          metadata: expect.objectContaining({ userId: "user-1" }),
        }),
      }),
      { idempotencyKey: "support-checkout-invite-1" }
    );
    expect(updateInvitation).toHaveBeenCalledWith({
      where: { id: "invite-1" },
      data: { userId: "user-1", checkoutSessionId: "cs_test" },
    });
  });
});
