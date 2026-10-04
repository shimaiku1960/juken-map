import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 差し替えるのは外部境界（認証）だけ。
vi.mock("../session-store.ts", () => ({ findSession: vi.fn() }));

const { findSession } = await import("../session-store.ts");
const { registerLineRoutes } = await import("./line.ts");
const { buildTestApp, loggedInSession, request } = await import("../test-support.ts");

const getSession = findSession as unknown as Mock;

const app = buildTestApp(registerLineRoutes);

// WEB_ORIGIN 未設定なので webOrigin() は SITE_URL を返す。
const ORIGIN = "https://juken-map.com";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("GET /line/settings", () => {
  it("ログイン済みならプロフィールの通知設定へ移動する", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await request(app, "GET", "/line/settings");

    expect(res.statusCode).toBe(302);
    expect(res.headers.location).toBe(
      `${ORIGIN}/profile#notification-settings`
    );
  });

  it("未ログインなら通知設定を戻り先にしてログインへ移動する", async () => {
    getSession.mockResolvedValue(null);

    const res = await request(app, "GET", "/line/settings");

    expect(res.statusCode).toBe(302);
    expect(res.headers.location).toBe(
      `${ORIGIN}/login?callbackURL=%2Fprofile%23notification-settings`
    );
  });
});
