import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { GET } from "./route";
import { auth } from "@/lib/auth";

vi.mock("@/lib/auth", () => ({ auth: { api: { getSession: vi.fn() } } }));
vi.mock("next/headers", () => ({ headers: vi.fn(() => new Headers()) }));

const getSession = auth.api.getSession as unknown as Mock;

beforeEach(() => vi.clearAllMocks());

describe("GET /line/settings", () => {
  it("ログイン済みならプロフィールの通知設定へ移動する", async () => {
    getSession.mockResolvedValue({ user: { id: "user-1" } });

    const response = await GET();

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://juken-map.com/profile#notification-settings"
    );
  });

  it("未ログインなら通知設定を戻り先にしてログインへ移動する", async () => {
    getSession.mockResolvedValue(null);

    const response = await GET();

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://juken-map.com/login?callbackURL=%2Fprofile%23notification-settings"
    );
  });
});
