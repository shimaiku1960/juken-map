import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 認証だけ差し替え、DB は本物のテスト用 MySQL に流す。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerTextbookRoutes } = await import("./textbooks.ts");
const { registerTextbookMasterRoutes } = await import("./textbook-masters.ts");
const { buildTestApp, request, demoSession } = await import("../test-support.ts");
const { cleanup, createTextbook, createTextbookMaster, createUser, findTextbooks } =
  await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp((app) => {
  registerTextbookRoutes(app);
  registerTextbookMasterRoutes(app);
});

const post = (body: unknown) => request(app, "POST", "/api/textbooks", body);
const patch = (id: number, body: unknown) =>
  request(app, "PATCH", `/api/textbooks/${id}`, body);

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("GET /api/textbooks", () => {
  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await request(app, "GET", "/api/textbooks")).statusCode).toBe(401);
  });

  it("自分の参考書だけを名前順で返す", async () => {
    await createTextbook(owner.id, { name: "物理のエッセンス" });
    await createTextbook(owner.id, { name: "青チャート" });
    const other = await createUser();
    await createTextbook(other.id, { name: "他人の参考書" });

    const res = await request(app, "GET", "/api/textbooks");

    expect(res.statusCode).toBe(200);
    expect(res.json().map((t: { name: string }) => t.name)).toEqual(["物理のエッセンス", "青チャート"]);
  });
});

describe("GET /api/textbook-masters", () => {
  it("マスターごとに総量の候補を id 順で返す", async () => {
    const masterId = await createTextbookMaster({
      name: "システム英単語",
      metrics: [
        { unit: "number", totalAmount: 2021, isDefault: true },
        { unit: "page", totalAmount: 400 },
      ],
    });

    const res = await request(app, "GET", "/api/textbook-masters");

    expect(res.statusCode).toBe(200);
    const master = res.json().find((m: { id: number }) => m.id === masterId);
    expect(master).toMatchObject({ name: "システム英単語" });
    expect(master.metrics).toEqual([
      expect.objectContaining({ unit: "number", totalAmount: 2021, isDefault: true }),
      expect.objectContaining({ unit: "page", totalAmount: 400, isDefault: false }),
    ]);
  });
});

describe("POST /api/textbooks", () => {
  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue(demoSession);

    expect((await post({ name: "英単語" })).statusCode).toBe(403);
  });

  it("名前から登録し、201で作った参考書を返す", async () => {
    const res = await post({ name: "  英単語  ", subject: "english", rangeUnit: "page" });

    expect(res.statusCode).toBe(201);
    expect(res.json()).toMatchObject({
      userId: owner.id,
      name: "英単語",
      subject: "english",
      rangeUnit: "page",
      masterId: null,
      totalAmount: null,
      targetDate: null,
    });
    expect(await findTextbooks(owner.id)).toHaveLength(1);
  });

  it("同じ名前は409を返し、2つ目を作らない", async () => {
    await post({ name: "英単語" });

    const res = await post({ name: "英単語" });

    expect(res.statusCode).toBe(409);
    expect(await findTextbooks(owner.id)).toHaveLength(1);
  });

  it("別のユーザーなら同じ名前でも登録できる", async () => {
    const other = await createUser();
    await createTextbook(other.id, { name: "英単語" });

    expect((await post({ name: "英単語" })).statusCode).toBe(201);
  });

  it("マスターから登録すると、既定の総量を使う", async () => {
    const masterId = await createTextbookMaster({
      name: "ターゲット1900",
      metrics: [
        { unit: "page", totalAmount: 500 },
        { unit: "number", totalAmount: 1900, isDefault: true },
      ],
    });

    const res = await post({ masterId });

    expect(res.statusCode).toBe(201);
    expect(res.json()).toMatchObject({
      name: "ターゲット1900",
      masterId,
      totalAmount: 1900,
      rangeUnit: "number",
    });
  });

  it("既定の総量が無ければ、先頭（id が最小）の総量を使う", async () => {
    const masterId = await createTextbookMaster({
      metrics: [
        { unit: "question", totalAmount: 300 },
        { unit: "page", totalAmount: 250 },
      ],
    });

    const res = await post({ masterId });

    expect(res.json()).toMatchObject({ totalAmount: 300, rangeUnit: "question" });
  });

  it("総量が1つも無いマスターは400、存在しないマスターは404を返す", async () => {
    const empty = await createTextbookMaster();

    expect((await post({ masterId: empty })).statusCode).toBe(400);
    expect((await post({ masterId: 999999999 })).statusCode).toBe(404);
    expect(await findTextbooks(owner.id)).toHaveLength(0);
  });
});

describe("PATCH /api/textbooks/:id", () => {
  it("送られた項目だけを更新する", async () => {
    const id = await createTextbook(owner.id, { name: "英単語", totalAmount: 100, rangeUnit: "page" });

    const res = await patch(id, { targetDate: "2027-01-31" });

    expect(res.statusCode).toBe(200);
    // 目標日は UTC の 0 時として保存される。ほかの項目は変わらない
    expect(res.json()).toMatchObject({
      targetDate: "2027-01-31T00:00:00.000Z",
      totalAmount: 100,
      rangeUnit: "page",
    });
  });

  it("目標日と科目は null で消せる", async () => {
    const id = await createTextbook(owner.id);
    await patch(id, { targetDate: "2027-01-31", subject: "english" });

    const res = await patch(id, { targetDate: null, subject: null });

    expect(res.json()).toMatchObject({ targetDate: null, subject: null });
  });

  it("他人の参考書なら404を返し、変更しない", async () => {
    const other = await createUser();
    const id = await createTextbook(other.id, { totalAmount: 100 });

    const res = await patch(id, { totalAmount: 5 });

    expect(res.statusCode).toBe(404);
    expect((await findTextbooks(other.id))[0].totalAmount).toBe(100);
  });

  it("IDが数値でなければ400を返す", async () => {
    expect((await request(app, "PATCH", "/api/textbooks/abc", {})).statusCode).toBe(400);
  });
});
