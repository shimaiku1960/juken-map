import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services/simulation-service", () => ({
  getSimulationState: vi.fn(),
  markSimulationUser: vi.fn(),
  updateSimulationUser: vi.fn(),
}));

const service = await import("@/api/services/simulation-service");
const { registerSimRoutes } = await import("./sim.ts");
const { buildTestApp, request } = await import("../test-support.ts");

const SIM = "delivered+sim00001@resend.dev";

// 登録するかどうかは起動時の環境変数で決まるので、テストごとにアプリを作り直す。
function appWith(env: { enabled?: string; secret?: string }) {
  process.env.SIMULATION_ENABLED = env.enabled;
  process.env.SIMULATION_SECRET = env.secret;
  if (env.enabled === undefined) delete process.env.SIMULATION_ENABLED;
  if (env.secret === undefined) delete process.env.SIMULATION_SECRET;
  return buildTestApp(registerSimRoutes);
}

const auth = (secret: string) => ({ authorization: `Bearer ${secret}` });

beforeEach(() => {
  vi.clearAllMocks();
});

describe("/api/sim/*", () => {
  it("SIMULATION_ENABLED が無ければルート自体が無い（404）", async () => {
    const app = appWith({ secret: "s" });
    const res = await request(app, "GET", "/api/sim/state", undefined, auth("s"));
    expect(res.statusCode).toBe(404);
  });

  it("秘密値が一致しなければ401", async () => {
    const app = appWith({ enabled: "on", secret: "s" });
    const res = await request(app, "GET", "/api/sim/state", undefined, auth("wrong"));
    expect(res.statusCode).toBe(401);
    expect(service.getSimulationState).not.toHaveBeenCalled();
  });

  it("秘密値が未設定なら、何を送っても401", async () => {
    const app = appWith({ enabled: "on" });
    const res = await request(app, "GET", "/api/sim/state", undefined, auth("undefined"));
    expect(res.statusCode).toBe(401);
  });

  it("状態を返す", async () => {
    vi.mocked(service.getSimulationState).mockResolvedValue({ nextSeq: 1, users: [] });
    const app = appWith({ enabled: "on", secret: "s" });
    const res = await request(app, "GET", "/api/sim/state", undefined, auth("s"));
    expect(res.statusCode).toBe(200);
    expect(res.json()).toEqual({ nextSeq: 1, users: [] });
  });

  describe("POST /api/sim/users", () => {
    it("シミュレーション用のアドレス以外は400（実ユーザーにも seed の合成ユーザーにも触らない）", async () => {
      const app = appWith({ enabled: "on", secret: "s" });
      const res = await request(
        app,
        "POST",
        "/api/sim/users",
        { email: "synthetic00001@synthetic.juken-map.invalid", seq: 1, cohort: "steady" },
        auth("s")
      );
      expect(res.statusCode).toBe(400);
      expect(service.markSimulationUser).not.toHaveBeenCalled();
    });

    it("連番と型を付ける", async () => {
      vi.mocked(service.markSimulationUser).mockResolvedValue(true);
      const app = appWith({ enabled: "on", secret: "s" });
      const res = await request(
        app,
        "POST",
        "/api/sim/users",
        { email: SIM, seq: 1, cohort: "steady" },
        auth("s")
      );
      expect(res.statusCode).toBe(204);
      expect(service.markSimulationUser).toHaveBeenCalledWith(SIM, { seq: 1, cohort: "steady" });
    });
  });

  describe("PATCH /api/sim/users/:seq", () => {
    it("日付の形が違えば400", async () => {
      const app = appWith({ enabled: "on", secret: "s" });
      const res = await request(
        app,
        "PATCH",
        "/api/sim/users/1",
        { lastActedOn: "2026/09/18" },
        auth("s")
      );
      expect(res.statusCode).toBe(400);
    });

    it("見つからなければ404", async () => {
      vi.mocked(service.updateSimulationUser).mockResolvedValue(false);
      const app = appWith({ enabled: "on", secret: "s" });
      const res = await request(
        app,
        "PATCH",
        "/api/sim/users/9",
        { lastActedOn: "2026-09-18" },
        auth("s")
      );
      expect(res.statusCode).toBe(404);
    });
  });
});
