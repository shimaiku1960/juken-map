import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { POST } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { getStripe } from "@/lib/stripe";

vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/lib/demo", () => ({
  demoReadOnlyGuard: vi.fn(() => null),
}));

vi.mock("@/lib/prisma", () => ({
  default: {
    supportSubscription: { findUnique: vi.fn() },
  },
}));

vi.mock("@/lib/stripe", () => ({
  getStripe: vi.fn(),
}));

vi.mock("@/lib/site", () => ({
  SITE_URL: "http://localhost:3000",
}));

vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findSubscription = prisma.supportSubscription.findUnique as unknown as Mock;
const createPortalSession = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getStripe).mockReturnValue({
    billingPortal: { sessions: { create: createPortalSession } },
  } as unknown as ReturnType<typeof getStripe>);
  createPortalSession.mockResolvedValue({
    url: "https://billing.stripe.test/session",
  });
});

describe("POST /api/support/portal", () => {
  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    const response = await POST();

    expect(response.status).toBe(401);
    expect(createPortalSession).not.toHaveBeenCalled();
  });

  it("契約がなければ404を返す", async () => {
    getSession.mockResolvedValue({ user: { id: "user-1" } });
    findSubscription.mockResolvedValue(null);

    const response = await POST();

    expect(response.status).toBe(404);
    expect(createPortalSession).not.toHaveBeenCalled();
  });

  it("ログインユーザーのStripe顧客用Portal Sessionを作成する", async () => {
    getSession.mockResolvedValue({ user: { id: "user-1" } });
    findSubscription.mockResolvedValue({ stripeCustomerId: "cus_test" });

    const response = await POST();

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      url: "https://billing.stripe.test/session",
    });
    expect(findSubscription).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      select: { stripeCustomerId: true },
    });
    expect(createPortalSession).toHaveBeenCalledWith({
      customer: "cus_test",
      return_url: "http://localhost:3000/dashboard",
      locale: "ja",
    });
  });
});
