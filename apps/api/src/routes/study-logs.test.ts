import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 認証だけ差し替え、DB は本物のテスト用 MySQL に流す。
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
const { registerStudyLogRoutes } = await import("./study-logs.ts");
const { buildTestApp, request, demoSession } = await import("../test-support.ts");
const {
  cleanup,
  createStudyLog,
  createTextbook,
  createUser,
  findStudyLogs,
  findUser,
} = await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerStudyLogRoutes);

const get = () => request(app, "GET", "/api/study-logs");
const post = (body: unknown) => request(app, "POST", "/api/study-logs", body);

const validBody = { date: "2026-02-20", minutes: 60, subject: "english" };

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("GET /api/study-logs", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await get()).statusCode).toBe(401);
  });

  it("自分の実績だけを、参考書を付けて新しい日付順（同じ日は記録した順）で返す", async () => {
    const textbookId = await createTextbook(owner.id, { name: "英単語", totalAmount: 300 });
    const older = await createStudyLog(owner.id, { date: new Date("2026-02-19T00:00:00.000Z") });
    const sameDayFirst = await createStudyLog(owner.id, {
      date: new Date("2026-02-20T00:00:00.000Z"),
      minutes: 60,
      subject: "english",
      textbookId,
    });
    const sameDaySecond = await createStudyLog(owner.id, { date: new Date("2026-02-20T00:00:00.000Z") });
    const other = await createUser();
    await createStudyLog(other.id);

    const res = await get();

    expect(res.statusCode).toBe(200);
    const body = res.json();
    expect(body.map((log: { id: number }) => log.id)).toEqual([sameDayFirst, sameDaySecond, older]);
    // 日付は ISO 文字列で返る（クライアントはこの形を期待する）
    expect(body[0]).toEqual({
      id: sameDayFirst,
      userId: owner.id,
      date: "2026-02-20T00:00:00.000Z",
      minutes: 60,
      subject: "english",
      textbookId,
      textbook: {
        id: textbookId,
        masterId: null,
        name: "英単語",
        totalAmount: 300,
        rangeUnit: null,
        targetDate: null,
        subject: null,
      },
      rangeStart: null,
      rangeEnd: null,
      rangeUnit: null,
      memo: null,
      studyPlanId: null,
      createdAt: expect.stringMatching(/Z$/),
      updatedAt: expect.stringMatching(/Z$/),
    });
    expect(body[2].textbook).toBeNull();
  });
});

describe("POST /api/study-logs", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await post(validBody)).statusCode).toBe(401);
  });

  it("デモアカウントなら 403 を返す（作成しない）", async () => {
    getSession.mockResolvedValue(demoSession);

    expect((await post(validBody)).statusCode).toBe(403);
  });

  it("入力が不正なら 400 を返す（minutes 欠落）", async () => {
    const res = await post({ date: "2026-02-20" });

    expect(res.statusCode).toBe(400);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });

  it("未来日の実績なら 400 を返す", async () => {
    const res = await post({ ...validBody, date: "2999-01-01" });

    expect(res.statusCode).toBe(400);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });

  it("他人の参考書IDなら 400 を返す（所有チェック）", async () => {
    const other = await createUser();
    const othersTextbook = await createTextbook(other.id);

    const res = await post({ ...validBody, textbookId: othersTextbook });

    expect(res.statusCode).toBe(400);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });

  it("逆算設定と実績の単位が異なるなら 400 を返す", async () => {
    const textbookId = await createTextbook(owner.id, { totalAmount: 300, rangeUnit: "page" });

    const res = await post({
      ...validBody,
      textbookId,
      rangeStart: 1,
      rangeEnd: 10,
      rangeUnit: "question",
    });

    expect(res.statusCode).toBe(400);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });

  it("実績の終了位置が参考書の総量を超えたら 400 を返す", async () => {
    const textbookId = await createTextbook(owner.id, { totalAmount: 300, rangeUnit: "page" });

    const res = await post({
      ...validBody,
      textbookId,
      rangeStart: 290,
      rangeEnd: 301,
      rangeUnit: "page",
    });

    expect(res.statusCode).toBe(400);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });

  it("正常なら 201 で作成した実績を返し、最初の1件は初回記録として印を付ける", async () => {
    const textbookId = await createTextbook(owner.id, { totalAmount: 300, rangeUnit: "page" });

    const res = await post({
      ...validBody,
      textbookId,
      rangeStart: 1,
      rangeEnd: 10,
      rangeUnit: "page",
      memo: "復習",
    });

    expect(res.statusCode).toBe(201);
    const body = res.json();
    expect(body).toMatchObject({
      userId: owner.id,
      date: "2026-02-20T00:00:00.000Z",
      minutes: 60,
      subject: "english",
      textbookId,
      rangeStart: 1,
      rangeEnd: 10,
      rangeUnit: "page",
      memo: "復習",
      studyPlanId: null,
      isFirstStudyLog: true,
    });
    expect(await findStudyLogs(owner.id)).toEqual([expect.objectContaining({ id: body.id })]);
    expect((await findUser(owner.id))?.firstStudyLogAt).toBeInstanceOf(Date);
  });

  it("2件目は初回記録にせず、初回の日時も上書きしない", async () => {
    await post(validBody);
    const firstStudyLogAt = (await findUser(owner.id))?.firstStudyLogAt;

    const res = await post({ ...validBody, minutes: 15 });

    expect(res.json().isFirstStudyLog).toBe(false);
    expect((await findUser(owner.id))?.firstStudyLogAt).toEqual(firstStudyLogAt);
  });

  it("初めての記録が同時に2件来ても、初回扱いは1件だけ", async () => {
    const responses = await Promise.all([post(validBody), post(validBody)]);

    expect(responses.map((res) => res.statusCode)).toEqual([201, 201]);
    expect(responses.filter((res) => res.json().isFirstStudyLog)).toHaveLength(1);
    expect(await findStudyLogs(owner.id)).toHaveLength(2);
  });

  it("想定外のエラーは握りつぶさず 500 になる", async () => {
    (transaction as unknown as Mock).mockRejectedValueOnce(new Error("DB down"));

    const res = await post(validBody);

    expect(res.statusCode).toBe(500);
    expect(await findStudyLogs(owner.id)).toHaveLength(0);
  });
});
