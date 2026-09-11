import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 認証だけは差し替える（Better Auth のセッション発行はこのテストの関心ではない）。
// DB は本物のテスト用 MySQL に流す。SQL の誤りや制約の効き方はモックでは分からないため。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

// 本物の transaction をそのまま使い、「DB が落ちた」ときだけ失敗させられるように包む。
vi.mock("@/api/infra/db", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/infra/db")>();
  return { ...actual, transaction: vi.fn(actual.transaction) };
});

const { auth } = await import("../auth.ts");
const { transaction } = await import("@/api/infra/db");
const { registerStudyPlanRoutes } = await import("./study-plans.ts");
const { buildTestApp, request, demoSession } = await import("../test-support.ts");
const {
  cleanup,
  createStudyLog,
  createStudyPlan,
  createTextbook,
  createUser,
  findStudyLogs,
  findStudyPlan,
  findUser,
} = await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerStudyPlanRoutes);

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("GET /api/study-plans", () => {
  it("自分の予定だけを、参考書と実績の有無を付けて日付順に返す", async () => {
    const textbookId = await createTextbook(owner.id, { name: "英単語", totalAmount: 300 });
    const later = await createStudyPlan(owner.id, {
      date: new Date("2027-02-21T00:00:00.000Z"),
      textbookId,
    });
    const earlier = await createStudyPlan(owner.id, {
      date: new Date("2027-02-20T00:00:00.000Z"),
      content: "自由入力",
    });
    const logId = await createStudyLog(owner.id, { studyPlanId: later });
    const other = await createUser();
    await createStudyPlan(other.id);

    const res = await request(app, "GET", "/api/study-plans");

    expect(res.statusCode).toBe(200);
    const body = res.json();
    expect(body.map((plan: { id: number }) => plan.id)).toEqual([earlier, later]);
    expect(body[0]).toMatchObject({
      // DB に入れた UTC の値がそのまま返る（読み書きの時間帯がずれていれば9時間ずれる）
      date: "2027-02-20T00:00:00.000Z",
      content: "自由入力",
      done: false,
      textbook: null,
      studyLogId: null,
    });
    expect(body[1]).toMatchObject({
      date: "2027-02-21T00:00:00.000Z",
      textbookId,
      textbook: { id: textbookId, name: "英単語", totalAmount: 300 },
      studyLogId: logId,
    });
  });
});

describe("PATCH /api/study-plans/:id", () => {
  it("実績記録済みの予定は未完了に戻せない", async () => {
    const planId = await createStudyPlan(owner.id, { done: true });
    await createStudyLog(owner.id, { studyPlanId: planId });

    const res = await request(app, "PATCH", `/api/study-plans/${planId}`, { done: false });

    expect(res.statusCode).toBe(409);
    expect((await findStudyPlan(planId))?.done).toBe(true);
  });

  it("実績がなければ予定のみ未完了へ戻せる", async () => {
    const planId = await createStudyPlan(owner.id, { done: true });

    const res = await request(app, "PATCH", `/api/study-plans/${planId}`, { done: false });

    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ id: planId, done: false });
    expect((await findStudyPlan(planId))?.done).toBe(false);
  });

  it("他人の予定なら404を返し、変更しない", async () => {
    const other = await createUser();
    const planId = await createStudyPlan(other.id, { content: "元のまま" });

    const res = await request(app, "PATCH", `/api/study-plans/${planId}`, { content: "書き換え" });

    expect(res.statusCode).toBe(404);
    expect((await findStudyPlan(planId))?.content).toBe("元のまま");
  });
});

describe("POST /api/study-plans/:id/complete", () => {
  const complete = (planId: number, body: unknown) =>
    request(app, "POST", `/api/study-plans/${planId}/complete`, body);

  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);
    const planId = await createStudyPlan(owner.id);

    const res = await complete(planId, { minutes: 30 });

    expect(res.statusCode).toBe(401);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });

  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue(demoSession);
    const planId = await createStudyPlan(owner.id);

    const res = await complete(planId, { minutes: 30 });

    expect(res.statusCode).toBe(403);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });

  it("学習時間が不正なら400を返す", async () => {
    const planId = await createStudyPlan(owner.id);

    const res = await complete(planId, { minutes: 0 });

    expect(res.statusCode).toBe(400);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });

  it("他人の予定なら404を返す", async () => {
    const other = await createUser();
    const planId = await createStudyPlan(other.id);

    const res = await complete(planId, { minutes: 30 });

    expect(res.statusCode).toBe(404);
    expect(await findStudyLogs(other.id)).toHaveLength(0);
    expect((await findStudyPlan(planId))?.done).toBe(false);
  });

  it("実績を記録済みなら409を返す", async () => {
    const planId = await createStudyPlan(owner.id);
    await createStudyLog(owner.id, { studyPlanId: planId });

    const res = await complete(planId, { minutes: 30 });

    expect(res.statusCode).toBe(409);
    expect(await findStudyLogs(owner.id)).toHaveLength(1);
  });

  it("実績の作成・予定の完了・初回記録の印を1回で行う", async () => {
    const textbookId = await createTextbook(owner.id, { totalAmount: 300, rangeUnit: "page" });
    const planId = await createStudyPlan(owner.id, {
      date: new Date("2027-02-20T00:00:00.000Z"),
      subject: "english",
      textbookId,
      rangeStart: 10,
      rangeEnd: 20,
      rangeUnit: "page",
    });
    const startedAt = Date.now();

    const res = await complete(planId, {
      minutes: 45,
      rangeStart: 12,
      rangeEnd: 18,
      rangeUnit: "page",
      memo: "復習した",
    });

    expect(res.statusCode).toBe(201);
    const body = res.json();
    expect(body.isFirstStudyLog).toBe(true);
    expect(body.plan).toMatchObject({ id: planId, done: true });
    expect(body.log).toMatchObject({
      userId: owner.id,
      studyPlanId: planId,
      date: "2027-02-20T00:00:00.000Z",
      minutes: 45,
      subject: "english",
      textbookId,
      rangeStart: 12,
      rangeEnd: 18,
      rangeUnit: "page",
      memo: "復習した",
      textbook: { id: textbookId, totalAmount: 300, rangeUnit: "page" },
    });

    // 応答だけでなく、DB に3つとも残っていることを確かめる
    expect((await findStudyPlan(planId))?.done).toBe(true);
    expect(await findStudyLogs(owner.id)).toHaveLength(1);
    const firstStudyLogAt = (await findUser(owner.id))?.firstStudyLogAt;
    expect(Math.abs(firstStudyLogAt!.getTime() - startedAt)).toBeLessThan(60_000);
  });

  it("2件目の記録は初回扱いにせず、初回記録の日時も上書きしない", async () => {
    const first = await createStudyPlan(owner.id);
    const second = await createStudyPlan(owner.id);
    await complete(first, { minutes: 30 });
    const firstStudyLogAt = (await findUser(owner.id))?.firstStudyLogAt;

    const res = await complete(second, { minutes: 30 });

    expect(res.statusCode).toBe(201);
    expect(res.json().isFirstStudyLog).toBe(false);
    expect((await findUser(owner.id))?.firstStudyLogAt).toEqual(firstStudyLogAt);
  });

  it("同じ予定を同時に完了しても実績は1件だけで、もう片方は409になる", async () => {
    const planId = await createStudyPlan(owner.id);

    // 両方が「まだ実績が無い」を読んだあとに INSERT すると、一意制約が片方を止める。
    const responses = await Promise.all([
      complete(planId, { minutes: 30 }),
      complete(planId, { minutes: 30 }),
    ]);

    expect(responses.map((res) => res.statusCode).sort()).toEqual([201, 409]);
    expect(responses.find((res) => res.statusCode === 409)?.json()).toEqual({
      error: "この予定の実績はすでに記録されています",
    });
    expect(await findStudyLogs(owner.id)).toHaveLength(1);
  });

  it("想定外のtransactionエラーは握りつぶさず500になる", async () => {
    const planId = await createStudyPlan(owner.id);
    (transaction as unknown as Mock).mockRejectedValueOnce(new Error("DB down"));

    const res = await complete(planId, { minutes: 30 });

    expect(res.statusCode).toBe(500);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });
});
