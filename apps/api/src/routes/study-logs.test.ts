import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";

// 外部境界だけを差し替える（DB・認証・サービス層）。門番ロジック本体は実物を動かす。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/backend/infra/prisma", () => ({
  prisma: {
    user: { updateMany: vi.fn() },
    studyLog: { create: vi.fn() },
    textbook: { findFirst: vi.fn() },
    $transaction: vi.fn(),
  },
}));

vi.mock("@/backend/services/study-log-service", () => ({
  listStudyLogs: vi.fn(),
}));

const { auth } = await import("../auth.ts");
const { prisma } = await import("@/backend/infra/prisma");
const { listStudyLogs } = await import("@/backend/services/study-log-service");
const { registerStudyLogRoutes } = await import("./study-logs.ts");
const { buildTestApp, request, loggedInSession, demoSession } = await import(
  "../test-support.ts"
);

const getSession = auth.api.getSession as unknown as Mock;
const list = listStudyLogs as unknown as Mock;
const create = prisma.studyLog.create as unknown as Mock;
const markActivation = prisma.user.updateMany as unknown as Mock;
const transaction = prisma.$transaction as unknown as Mock;
const findTextbook = prisma.textbook.findFirst as unknown as Mock;

const app = buildTestApp(registerStudyLogRoutes);

const get = () => request(app, "GET", "/api/study-logs");
const post = (body: unknown) => request(app, "POST", "/api/study-logs", body);

const validBody = { date: "2026-02-20", minutes: 60, subject: "english" };

beforeEach(() => {
  vi.clearAllMocks();
  markActivation.mockResolvedValue({ count: 0 });
  transaction.mockImplementation((callback: (tx: unknown) => unknown) =>
    callback(prisma)
  );
});

describe("GET /api/study-logs", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await get();

    expect(res.statusCode).toBe(401);
    expect(list).not.toHaveBeenCalled();
  });

  it("ログイン済みなら自分の実績を 200 で返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    // Prisma は必ず全スカラー列を返すので、モックも実際の行に揃える。
    // GET は DTO 変換（Date → ISO 文字列）を通すため、部分的な行では再現にならない。
    list.mockResolvedValue([
      {
        id: 1,
        userId: "user-1",
        date: new Date("2026-02-20T00:00:00.000Z"),
        minutes: 60,
        subject: "english",
        textbookId: null,
        textbook: null,
        rangeStart: null,
        rangeEnd: null,
        rangeUnit: null,
        memo: null,
        studyPlanId: null,
        createdAt: new Date("2026-02-20T01:00:00.000Z"),
        updatedAt: new Date("2026-02-20T02:00:00.000Z"),
      },
    ]);

    const res = await get();

    expect(res.statusCode).toBe(200);
    // 日付が ISO 文字列になって返ることまで検証する（クライアントはこの形を期待する）。
    expect(res.json()).toEqual([
      {
        id: 1,
        userId: "user-1",
        date: "2026-02-20T00:00:00.000Z",
        minutes: 60,
        subject: "english",
        textbookId: null,
        textbook: null,
        rangeStart: null,
        rangeEnd: null,
        rangeUnit: null,
        memo: null,
        studyPlanId: null,
        createdAt: "2026-02-20T01:00:00.000Z",
        updatedAt: "2026-02-20T02:00:00.000Z",
      },
    ]);
    expect(list).toHaveBeenCalledWith("user-1");
  });
});

describe("POST /api/study-logs", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    const res = await post(validBody);

    expect(res.statusCode).toBe(401);
    expect(create).not.toHaveBeenCalled();
  });

  it("デモアカウントなら 403 を返す（作成しない）", async () => {
    getSession.mockResolvedValue(demoSession);

    const res = await post(validBody);

    expect(res.statusCode).toBe(403);
    expect(create).not.toHaveBeenCalled();
  });

  it("入力が不正なら 400 を返す（minutes 欠落）", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await post({ date: "2026-02-20" });

    expect(res.statusCode).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("未来日の実績なら 400 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await post({ ...validBody, date: "2999-01-01" });

    expect(res.statusCode).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("他人の参考書IDなら 400 を返す（所有チェック）", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findTextbook.mockResolvedValue(null); // 自分の所有分に無い

    const res = await post({ ...validBody, textbookId: 999 });

    expect(res.statusCode).toBe(400);
    expect(create).not.toHaveBeenCalled();
    expect(findTextbook).toHaveBeenCalledWith(
      expect.objectContaining({ where: { id: 999, userId: "user-1" } })
    );
  });

  it("逆算設定と実績の単位が異なるなら 400 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findTextbook.mockResolvedValue({
      id: 3,
      userId: "user-1",
      totalAmount: 300,
      rangeUnit: "page",
    });

    const res = await post({
      ...validBody,
      textbookId: 3,
      rangeStart: 1,
      rangeEnd: 10,
      rangeUnit: "question",
    });

    expect(res.statusCode).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("実績の終了位置が参考書の総量を超えたら 400 を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    findTextbook.mockResolvedValue({
      id: 3,
      userId: "user-1",
      totalAmount: 300,
      rangeUnit: "page",
    });

    const res = await post({
      ...validBody,
      textbookId: 3,
      rangeStart: 290,
      rangeEnd: 301,
      rangeUnit: "page",
    });

    expect(res.statusCode).toBe(400);
    expect(create).not.toHaveBeenCalled();
  });

  it("正常なら 201 で作成した実績を返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    const created = { id: 5, minutes: 60, userId: "user-1" };
    create.mockResolvedValue(created);

    const res = await post(validBody);

    expect(res.statusCode).toBe(201);
    expect(res.json()).toEqual({ ...created, isFirstStudyLog: false });
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ minutes: 60, userId: "user-1" }),
      })
    );
  });

  it("最初の実績なら初回記録として返す", async () => {
    getSession.mockResolvedValue(loggedInSession);
    markActivation.mockResolvedValue({ count: 1 });
    create.mockResolvedValue({ id: 5, minutes: 60, userId: "user-1" });

    const res = await post(validBody);

    expect(res.statusCode).toBe(201);
    expect(res.json()).toEqual(
      expect.objectContaining({ isFirstStudyLog: true })
    );
    expect(markActivation).toHaveBeenCalledWith({
      where: { id: "user-1", firstStudyLogAt: null },
      data: { firstStudyLogAt: expect.any(Date) },
    });
  });

  it("想定外のエラーは握りつぶさず 500 になる", async () => {
    getSession.mockResolvedValue(loggedInSession);
    create.mockRejectedValue(new Error("DB down"));

    const res = await post(validBody);

    expect(res.statusCode).toBe(500);
  });
});
