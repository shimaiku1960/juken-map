import { randomUUID } from "node:crypto";
import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerAdminRoutes } = await import("./admin.ts");
const { buildTestApp, request, loggedInSession, takeLogLines } = await import("../test-support.ts");
const { execute } = await import("@/api/infra/db");
const { setUserRoleByEmail } = await import("@/api/services/user-service");
const { cleanup, createAccount, createSession, createStudyLog, createStudyPlan, createUser } =
  await import("../test-db/fixtures.ts");
const { select } = await import("@/api/infra/db");
const { DEMO_EMAIL } = await import("@/shared/demo");

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
  takeLogLines();
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

  // 停止・削除は影響が大きいので、読み取りとは別に否定のテストを持つ。
  it.each([
    ["POST", "/api/admin/users/someone/ban"],
    ["POST", "/api/admin/users/someone/unban"],
    ["DELETE", "/api/admin/users/someone"],
  ] as const)("%s %s は未ログインなら401、一般ユーザーなら403", async (method, url) => {
    getSession.mockResolvedValue(null);
    expect((await request(app, method, url)).statusCode).toBe(401);

    getSession.mockResolvedValue({ user: { ...loggedInSession.user, role: "user" } });
    expect((await request(app, method, url)).statusCode).toBe(403);
  });
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

describe("停止・解除・削除", () => {
  // 管理者本人として振る舞うため、操作する側の id を都度入れ替える。
  const actAs = (id: string) => getSession.mockResolvedValue({ user: { id, email: "a@example.com", role: "admin" } });

  const bannedAt = async (id: string) => {
    const [row] = await select<{ bannedAt: Date | null }>(
      "SELECT bannedAt FROM `user` WHERE id = ?",
      [id]
    );
    return row?.bannedAt ?? null;
  };

  it("停止すると bannedAt が入り、その人の session が消える", async () => {
    const user = await createUser();
    await createSession(user.id);
    await createSession(user.id);

    const res = await request(app, "POST", `/api/admin/users/${user.id}/ban`);

    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ id: user.id, sessionsRemoved: 2 });
    expect(await bannedAt(user.id)).not.toBeNull();
    const [{ remaining }] = await select<{ remaining: number }>(
      "SELECT COUNT(*) AS remaining FROM session WHERE userId = ?",
      [user.id]
    );
    expect(Number(remaining)).toBe(0);
  });

  it("停止を押し直しても最初に止めた日時は変わらない", async () => {
    const user = await createUser();
    await request(app, "POST", `/api/admin/users/${user.id}/ban`);
    const first = await bannedAt(user.id);

    await request(app, "POST", `/api/admin/users/${user.id}/ban`);

    expect(await bannedAt(user.id)).toEqual(first);
  });

  it("解除すると bannedAt が NULL に戻る", async () => {
    const user = await createUser();
    await request(app, "POST", `/api/admin/users/${user.id}/ban`);

    const res = await request(app, "POST", `/api/admin/users/${user.id}/unban`);

    expect(res.statusCode).toBe(200);
    expect(await bannedAt(user.id)).toBeNull();
  });

  it("自分自身は停止できない", async () => {
    const user = await createUser();
    actAs(user.id);

    const res = await request(app, "POST", `/api/admin/users/${user.id}/ban`);

    expect(res.statusCode).toBe(409);
    expect(await bannedAt(user.id)).toBeNull();
  });

  it("他の管理者は停止できない", async () => {
    const user = await createUser();
    await execute("UPDATE `user` SET role = 'admin' WHERE id = ?", [user.id]);

    const res = await request(app, "POST", `/api/admin/users/${user.id}/ban`);

    expect(res.statusCode).toBe(409);
    expect(await bannedAt(user.id)).toBeNull();
  });

  it("デモアカウントは停止できない", async () => {
    // 手元の DB には seed 済みのデモがいる。いなければ作る（どちらでも 409 で何も変わらない）。
    const [existing] = await select<{ id: string }>("SELECT id FROM `user` WHERE email = ?", [
      DEMO_EMAIL,
    ]);
    let demoId = existing?.id;
    if (!demoId) {
      const user = await createUser();
      await execute("UPDATE `user` SET email = ? WHERE id = ?", [DEMO_EMAIL, user.id]);
      demoId = user.id;
    }

    expect((await request(app, "POST", `/api/admin/users/${demoId}/ban`)).statusCode).toBe(409);
  });

  it("誰が誰に何をしたかを監査ログに残す", async () => {
    const user = await createUser();

    await request(app, "POST", `/api/admin/users/${user.id}/ban`);
    await request(app, "POST", `/api/admin/users/${user.id}/unban`);

    const logs = takeLogLines().filter((line) => line.msg === "admin user action");
    expect(logs.map((line) => line.action)).toEqual(["ban", "unban"]);
    expect(logs[0]).toMatchObject({
      adminId: "admin-1",
      targetId: user.id,
      targetEmail: user.session.user.email,
    });
  });

  it("いないユーザーは404", async () => {
    expect((await request(app, "POST", `/api/admin/users/${randomUUID()}/ban`)).statusCode).toBe(404);
  });

  it("削除はメールアドレスが一致して初めて消え、学習データも一緒に消える", async () => {
    const user = await createUser();
    await createStudyLog(user.id);
    await createStudyPlan(user.id);

    const wrong = await request(app, "DELETE", `/api/admin/users/${user.id}`, {
      email: "other@example.test",
    });
    expect(wrong.statusCode).toBe(400);

    const res = await request(app, "DELETE", `/api/admin/users/${user.id}`, {
      email: user.session.user.email,
    });

    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ removed: { studyLogs: 1, studyPlans: 1 } });
    const [{ remaining }] = await select<{ remaining: number }>(
      "SELECT COUNT(*) AS remaining FROM StudyLog WHERE userId = ?",
      [user.id]
    );
    expect(Number(remaining)).toBe(0);
  });

  it("削除もメールアドレスの大文字小文字は問わない", async () => {
    const user = await createUser();

    const res = await request(app, "DELETE", `/api/admin/users/${user.id}`, {
      email: user.session.user.email.toUpperCase(),
    });

    expect(res.statusCode).toBe(200);
  });

  it("自分自身は削除できない", async () => {
    const user = await createUser();
    actAs(user.id);

    const res = await request(app, "DELETE", `/api/admin/users/${user.id}`, {
      email: user.session.user.email,
    });

    expect(res.statusCode).toBe(409);
  });

  it("確認用のメールアドレスが無ければ400", async () => {
    const user = await createUser();
    expect((await request(app, "DELETE", `/api/admin/users/${user.id}`)).statusCode).toBe(400);
  });
});
