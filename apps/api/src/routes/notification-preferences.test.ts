import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/backend/infra/prisma", () => ({
  prisma: { notificationPreference: { upsert: vi.fn() } },
}));

vi.mock("@/backend/services/notification-service", () => ({
  findNotificationPreference: vi.fn(),
  findLineConnection: vi.fn(),
}));

const { auth } = await import("../auth.ts");
const { prisma } = await import("@/backend/infra/prisma");
const { findNotificationPreference, findLineConnection } = await import(
  "@/backend/services/notification-service"
);
const { registerNotificationPreferenceRoutes } = await import(
  "./notification-preferences.ts"
);
const { buildTestApp, request, loggedInSession, demoSession } = await import(
  "../test-support.ts"
);

const getSession = auth.api.getSession as unknown as Mock;
const findPreference = findNotificationPreference as unknown as Mock;
const findConnection = findLineConnection as unknown as Mock;
const upsert = prisma.notificationPreference.upsert as unknown as Mock;

const app = buildTestApp(registerNotificationPreferenceRoutes);

const get = () => request(app, "GET", "/api/notification-preferences");
const put = (body: unknown) =>
  request(app, "PUT", "/api/notification-preferences", body);

beforeEach(() => vi.clearAllMocks());

describe("GET /api/notification-preferences", () => {
  it("未設定なら両方OFFを返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findPreference.mockResolvedValue(null);

    const res = await get();

    expect(res.json()).toEqual({
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

    expect((await put({})).statusCode).toBe(401);
  });

  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue(demoSession);

    const res = await put({
      emailMorningEnabled: true,
      emailEveningEnabled: true,
      lineMorningEnabled: false,
      lineEveningEnabled: false,
    });

    expect(res.statusCode).toBe(403);
  });

  it("設定をupsertする", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const input = {
      emailMorningEnabled: true,
      emailEveningEnabled: false,
      lineMorningEnabled: false,
      lineEveningEnabled: false,
    };
    upsert.mockResolvedValue({
      morningEnabled: true,
      eveningEnabled: false,
      lineMorningEnabled: false,
      lineEveningEnabled: false,
    });

    const res = await put(input);

    expect(res.statusCode).toBe(200);
    expect(res.json()).toEqual(input);
    expect(upsert).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { userId: "user-1" },
        create: {
          userId: "user-1",
          morningEnabled: true,
          eveningEnabled: false,
          lineMorningEnabled: false,
          lineEveningEnabled: false,
        },
        update: {
          morningEnabled: true,
          eveningEnabled: false,
          lineMorningEnabled: false,
          lineEveningEnabled: false,
        },
      })
    );
  });

  it("未連携ではLINE通知を有効化できない", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findConnection.mockResolvedValue(null);

    const res = await put({
      emailMorningEnabled: false,
      emailEveningEnabled: false,
      lineMorningEnabled: true,
      lineEveningEnabled: false,
    });

    expect(res.statusCode).toBe(400);
    expect(upsert).not.toHaveBeenCalled();
  });
});
