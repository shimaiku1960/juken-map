import { randomUUID } from "node:crypto";
import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerAdminRoutes } = await import("./admin.ts");
const { buildTestApp, request, loggedInSession } = await import("../test-support.ts");
const { execute } = await import("@/api/infra/db");
const { setUserRoleByEmail } = await import("@/api/services/user-service");
const { cleanup, createAccount, createStudyLog, createUser } = await import(
  "../test-db/fixtures.ts"
);

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerAdminRoutes);

const adminSession = { user: { id: "admin-1", email: "admin@example.com", role: "admin" } };

type ListBody = {
  users: { id: string; kind: string; studyLogCount: number; providers: string[] }[];
  total: number;
};
const listUsers = async (query: string) => {
  const res = await request(app, "GET", `/api/admin/users?${query}`);
  expect(res.statusCode).toBe(200);
  return res.json() as ListBody;
};

beforeEach(() => {
  vi.clearAllMocks();
  getSession.mockResolvedValue(adminSession);
});

afterAll(cleanup);

describe("管理者ガード", () => {
  it.each(["/api/admin/overview", "/api/admin/users"])("%s は未ログインなら401", async (url) => {
    getSession.mockResolvedValue(null);
    expect((await request(app, "GET", url)).statusCode).toBe(401);
  });

  it.each(["/api/admin/overview", "/api/admin/users"])(
    "%s は role が admin でなければ403",
    async (url) => {
      getSession.mockResolvedValue({ user: { ...loggedInSession.user, role: "user" } });
      expect((await request(app, "GET", url)).statusCode).toBe(403);
    }
  );
});

describe("GET /api/admin/overview", () => {
  it("4つの種別を決まった順で返し、直近に記録した実ユーザーを数える", async () => {
    const user = await createUser();
    await createStudyLog(user.id);

    const res = await request(app, "GET", "/api/admin/overview");

    expect(res.statusCode).toBe(200);
    const body = res.json() as { kinds: { kind: string; activeLast7Days: number }[] };
    expect(body.kinds.map((row) => row.kind)).toEqual(["real", "sim", "seed", "demo"]);
    expect(body.kinds[0]!.activeLast7Days).toBeGreaterThanOrEqual(1);
  });
});

describe("GET /api/admin/users", () => {
  it("記録件数と認証方法を付けて返す", async () => {
    const user = await createUser();
    await createAccount(user.id, "credential");
    await createStudyLog(user.id);
    await createStudyLog(user.id);

    const body = await listUsers(`kind=real&q=${user.id}`);

    expect(body.total).toBe(1);
    expect(body.users[0]).toMatchObject({
      id: user.id,
      kind: "real",
      studyLogCount: 2,
      providers: ["credential"],
    });
  });

  it("合成ユーザーは実ユーザーに混ざらず、それぞれの種別に分かれる", async () => {
    const tag = randomUUID();
    const seed = await createUser();
    await execute("UPDATE `user` SET email = ? WHERE id = ?", [
      `${tag}-seed@synthetic.juken-map.invalid`,
      seed.id,
    ]);
    const sim = await createUser();
    await execute("UPDATE `user` SET email = ?, simSeq = ? WHERE id = ?", [
      `${tag}-sim@example.test`,
      // simSeq は一意。並列に走る他のテストとぶつからない大きな値にする。
      1_000_000 + Math.floor(Math.random() * 1_000_000_000),
      sim.id,
    ]);

    expect((await listUsers(`kind=real&q=${tag}`)).total).toBe(0);
    expect((await listUsers(`kind=seed&q=${tag}`)).users.map((u) => u.id)).toEqual([seed.id]);
    expect((await listUsers(`kind=sim&q=${tag}`)).users.map((u) => u.id)).toEqual([sim.id]);
  });

  it("検索語の % は文字として扱い、全件一致にしない", async () => {
    expect((await listUsers("kind=real&q=%25")).total).toBe(0);
  });

  it("不正な種別やページは400", async () => {
    expect((await request(app, "GET", "/api/admin/users?kind=admin")).statusCode).toBe(400);
    expect((await request(app, "GET", "/api/admin/users?page=0")).statusCode).toBe(400);
  });
});

describe("setUserRoleByEmail（pnpm admin:grant）", () => {
  it("メール確認前のユーザーは管理者にしない", async () => {
    const user = await createUser();

    expect(await setUserRoleByEmail(user.session.user.email, "admin")).toEqual({
      result: "unverified",
    });
  });

  it("確認済みなら管理者にし、--revoke で戻せる", async () => {
    const user = await createUser();
    await execute("UPDATE `user` SET emailVerified = true WHERE id = ?", [user.id]);

    expect(await setUserRoleByEmail(user.session.user.email, "admin")).toEqual({
      result: "updated",
      previous: "user",
    });
    expect(await setUserRoleByEmail(user.session.user.email, "user")).toEqual({
      result: "updated",
      previous: "admin",
    });
  });

  it("いないアドレスは not_found", async () => {
    expect(await setUserRoleByEmail(`${randomUUID()}@example.test`, "admin")).toEqual({
      result: "not_found",
    });
  });
});
