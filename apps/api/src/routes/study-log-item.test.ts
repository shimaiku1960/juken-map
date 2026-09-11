import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 認証だけ差し替え、DB は本物のテスト用 MySQL に流す。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerStudyLogItemRoutes } = await import("./study-log-item.ts");
const { buildTestApp, request, demoSession } = await import("../test-support.ts");
const {
  cleanup,
  createStudyLog,
  createStudyPlan,
  createTextbook,
  createUser,
  findStudyLog,
} = await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerStudyLogItemRoutes);

const patch = (id: number, body: unknown) =>
  request(app, "PATCH", `/api/study-logs/${id}`, body);
const remove = (id: number) => request(app, "DELETE", `/api/study-logs/${id}`);

const validBody = { date: "2026-02-20", minutes: 90, subject: "english" };

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("PATCH /api/study-logs/:id", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);
    const logId = await createStudyLog(owner.id, { minutes: 30 });

    const res = await patch(logId, validBody);

    expect(res.statusCode).toBe(401);
    expect((await findStudyLog(logId))?.minutes).toBe(30);
  });

  it("他人の実績は 404 を返し、変更しない", async () => {
    const other = await createUser();
    const logId = await createStudyLog(other.id, { minutes: 30 });

    const res = await patch(logId, validBody);

    expect(res.statusCode).toBe(404);
    expect((await findStudyLog(logId))?.minutes).toBe(30);
  });

  it("他人の参考書IDなら 400 を返す", async () => {
    const logId = await createStudyLog(owner.id);
    const other = await createUser();
    const othersTextbook = await createTextbook(other.id);

    const res = await patch(logId, { ...validBody, textbookId: othersTextbook });

    expect(res.statusCode).toBe(400);
    expect((await findStudyLog(logId))?.textbookId).toBeNull();
  });

  it("自分の実績なら入力内容で更新し、更新後の実績を返す", async () => {
    const logId = await createStudyLog(owner.id, { minutes: 30, memo: "前のメモ" });

    // 送られなかった項目は空（null）になる。フォームは毎回全項目を送る前提
    const res = await patch(logId, validBody);

    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({
      id: logId,
      date: "2026-02-20T00:00:00.000Z",
      minutes: 90,
      subject: "english",
      memo: null,
    });
    expect(await findStudyLog(logId)).toMatchObject({ minutes: 90, subject: "english", memo: null });
  });

  it("予定由来の実績は日付・科目・参考書の紐づきを維持する", async () => {
    const textbookId = await createTextbook(owner.id, { name: "数学" });
    const otherTextbook = await createTextbook(owner.id, { name: "英語" });
    const planId = await createStudyPlan(owner.id);
    const logId = await createStudyLog(owner.id, {
      studyPlanId: planId,
      date: new Date("2026-02-20T00:00:00.000Z"),
      subject: "math",
      textbookId,
    });

    const res = await patch(logId, {
      ...validBody,
      date: "2026-02-21",
      subject: "english",
      textbookId: otherTextbook,
    });

    expect(res.statusCode).toBe(200);
    expect(await findStudyLog(logId)).toMatchObject({
      date: new Date("2026-02-20T00:00:00.000Z"),
      subject: "math",
      textbookId,
      minutes: 90,
    });
  });

  it("時間だけの修正では、後から変わった参考書設定を過去の実績へ遡って当てはめない", async () => {
    // 記録したあとで参考書の総量を 5 に減らした、という状態（範囲の終わり 10 は今の総量を超える）
    const textbookId = await createTextbook(owner.id, { totalAmount: 5, rangeUnit: "page" });
    const logId = await createStudyLog(owner.id, {
      date: new Date("2026-02-20T00:00:00.000Z"),
      minutes: 60,
      subject: "english",
      textbookId,
      rangeStart: 1,
      rangeEnd: 10,
      rangeUnit: "page",
    });

    const res = await patch(logId, {
      date: "2026-02-20",
      minutes: 90,
      subject: "english",
      textbookId,
      rangeStart: 1,
      rangeEnd: 10,
      rangeUnit: "page",
    });

    expect(res.statusCode).toBe(200);
    expect((await findStudyLog(logId))?.minutes).toBe(90);
  });

  it("範囲を変えるなら、今の参考書設定で確かめる", async () => {
    const textbookId = await createTextbook(owner.id, { totalAmount: 5, rangeUnit: "page" });
    const logId = await createStudyLog(owner.id, {
      textbookId,
      rangeStart: 1,
      rangeEnd: 3,
      rangeUnit: "page",
    });

    const res = await patch(logId, {
      ...validBody,
      textbookId,
      rangeStart: 1,
      rangeEnd: 10,
      rangeUnit: "page",
    });

    expect(res.statusCode).toBe(400);
    expect((await findStudyLog(logId))?.rangeEnd).toBe(3);
  });
});

describe("DELETE /api/study-logs/:id", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);
    const logId = await createStudyLog(owner.id);

    expect((await remove(logId)).statusCode).toBe(401);
    expect(await findStudyLog(logId)).not.toBeNull();
  });

  it("デモアカウントなら 403 を返す（削除しない）", async () => {
    getSession.mockResolvedValue(demoSession);
    const logId = await createStudyLog(owner.id);

    expect((await remove(logId)).statusCode).toBe(403);
    expect(await findStudyLog(logId)).not.toBeNull();
  });

  it("他人の実績は 404 を返す（削除しない）", async () => {
    const other = await createUser();
    const logId = await createStudyLog(other.id);

    expect((await remove(logId)).statusCode).toBe(404);
    expect(await findStudyLog(logId)).not.toBeNull();
  });

  it("存在しない実績は 404 を返す", async () => {
    expect((await remove(999999999)).statusCode).toBe(404);
  });

  it("自分の実績なら 200 で削除する", async () => {
    const logId = await createStudyLog(owner.id);

    const res = await remove(logId);

    expect(res.statusCode).toBe(200);
    expect(await findStudyLog(logId)).toBeNull();
  });
});
