import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { GET, PUT } from "./route";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

vi.mock("@/lib/auth", () => ({
  auth: { api: { getSession: vi.fn() } },
}));
vi.mock("@/lib/prisma", () => ({
  default: {
    notificationPreference: { findUnique: vi.fn(), upsert: vi.fn() },
    lineConnection: { findUnique: vi.fn() },
  },
}));
vi.mock("next/headers", () => ({
  headers: vi.fn(() => new Headers()),
}));

const getSession = auth.api.getSession as unknown as Mock;
const findUnique = prisma.notificationPreference.findUnique as unknown as Mock;
const upsert = prisma.notificationPreference.upsert as unknown as Mock;
const session = { user: { id: "user-1", email: "me@example.com" } };
const request = (body: unknown) =>
  ({ json: async () => body } as unknown as Request);

beforeEach(() => vi.clearAllMocks());

describe("GET /api/notification-preferences", () => {
  it("未設定なら両方OFFを返す", async () => {
    getSession.mockResolvedValue(session);
    findUnique.mockResolvedValue(null);

    await expect((await GET()).json()).resolves.toEqual({
      emailMorningEnabled: false,
      emailEveningEnabled: false,
      lineMorningEnabled: false,
      lineEveningEnabled: false,
    });
  });
});

describe("PUT /api/notification-preferences", () => {
  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);
    expect((await PUT(request({}))).status).toBe(401);
  });

  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue({
      user: { id: "demo", email: "demo@juken-map.com" },
    });
    expect(
      (await PUT(request({ emailMorningEnabled: true, emailEveningEnabled: true, lineMorningEnabled: false, lineEveningEnabled: false }))).status
    ).toBe(403);
  });

  it("設定をupsertする", async () => {
    getSession.mockResolvedValue(session);
    const input = { emailMorningEnabled: true, emailEveningEnabled: false, lineMorningEnabled: false, lineEveningEnabled: false };
    upsert.mockResolvedValue({ morningEnabled: true, eveningEnabled: false, lineMorningEnabled: false, lineEveningEnabled: false });

    const response = await PUT(request(input));

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual(input);
    expect(upsert).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { userId: "user-1" },
        create: { userId: "user-1", morningEnabled: true, eveningEnabled: false, lineMorningEnabled: false, lineEveningEnabled: false },
        update: { morningEnabled: true, eveningEnabled: false, lineMorningEnabled: false, lineEveningEnabled: false },
      })
    );
  });

  it("未連携ではLINE通知を有効化できない", async () => {
    getSession.mockResolvedValue(session);
    vi.mocked(prisma.lineConnection.findUnique).mockResolvedValue(null);
    const response = await PUT(request({ emailMorningEnabled: false, emailEveningEnabled: false, lineMorningEnabled: true, lineEveningEnabled: false }));
    expect(response.status).toBe(400);
    expect(upsert).not.toHaveBeenCalled();
  });
});
