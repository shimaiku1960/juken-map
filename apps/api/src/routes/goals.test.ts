import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 認証だけ差し替え、DB は本物のテスト用 MySQL に流す。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

// 本物の transaction をそのまま使い、「途中で DB が落ちた」ときだけ失敗させられるように包む。
vi.mock("@/api/infra/db", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/infra/db")>();
  return { ...actual, transaction: vi.fn(actual.transaction) };
});

const { auth } = await import("../auth.ts");
const db = await import("@/api/infra/db");
const { registerGoalRoutes } = await import("./goals.ts");
const { registerHomeRoutes } = await import("./home.ts");
const { buildTestApp, request, demoSession } = await import("../test-support.ts");
const { cleanup, createFinalGoal, createTag, createUniversity, createUser, findFinalGoals } =
  await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const transaction = db.transaction as unknown as Mock;
const app = buildTestApp((app) => {
  registerGoalRoutes(app);
  registerHomeRoutes(app);
});

const post = (body: unknown) => request(app, "POST", "/api/goals", body);
const put = (id: number, body: unknown) => request(app, "PUT", `/api/goals/${id}`, body);
const patch = (id: number, body: unknown) => request(app, "PATCH", `/api/goals/${id}`, body);
const del = (id: number) => request(app, "DELETE", `/api/goals/${id}`);

let owner: Awaited<ReturnType<typeof createUser>>;
// 学部を3つ持つ大学（1つ目の学部にだけタグが2つ付く）
let faculties: number[];
let tagIds: number[];

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
  const suffix = crypto.randomUUID();
  tagIds = [await createTag(`法-${suffix}`), await createTag(`難関-${suffix}`)];
  const university = await createUniversity({
    name: `大学-${suffix}`,
    faculties: [{ name: "法学部", tagIds: [tagIds[1], tagIds[0]] }, { name: "経済学部" }, { name: "文学部" }],
  });
  faculties = university.facultyIds;
});

afterAll(cleanup);

describe("GET /api/goals", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await request(app, "GET", "/api/goals")).statusCode).toBe(401);
  });

  it("自分の志望校だけを、学部・大学・タグを入れ子にして登録順で返す", async () => {
    const later = await createFinalGoal(owner.id, faculties[1], {
      createdAt: new Date("2026-09-02T00:00:00.000Z"),
    });
    const earlier = await createFinalGoal(owner.id, faculties[0], {
      createdAt: new Date("2026-09-01T00:00:00.000Z"),
      isFirstChoice: true,
      note: "本命",
    });
    const other = await createUser();
    await createFinalGoal(other.id, faculties[2]);

    const res = await request(app, "GET", "/api/goals");

    expect(res.statusCode).toBe(200);
    const goals = res.json();
    expect(goals.map((g: { id: number }) => g.id)).toEqual([earlier, later]);
    expect(goals[0]).toMatchObject({
      userId: owner.id,
      facultyId: faculties[0],
      isFirstChoice: true,
      note: "本命",
      status: "decided",
      createdAt: "2026-09-01T00:00:00.000Z",
      faculty: {
        id: faculties[0],
        name: "法学部",
        examDate: "2027-02-15T00:00:00.000Z",
        university: { prefecture: "東京都", type: "私立" },
      },
    });
    // タグは id 順。付いていない学部は空配列
    expect(goals[0].faculty.tags.map((t: { id: number }) => t.id)).toEqual(tagIds);
    expect(goals[1].faculty.tags).toEqual([]);
  });
});

describe("GET /api/goals/first-choice", () => {
  it("受験校として確定した第一志望を、学部と大学を付けて返す", async () => {
    await createFinalGoal(owner.id, faculties[0]);
    const first = await createFinalGoal(owner.id, faculties[1], { isFirstChoice: true });

    const res = await request(app, "GET", "/api/goals/first-choice");

    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ id: first, faculty: { name: "経済学部", university: {} } });
    expect(res.json().faculty.tags).toBeUndefined();
  });

  it("第一志望が比較検討中（candidate）なら null を返す", async () => {
    await createFinalGoal(owner.id, faculties[0], { isFirstChoice: true, status: "candidate" });

    const res = await request(app, "GET", "/api/goals/first-choice");

    expect(res.statusCode).toBe(200);
    expect(res.json()).toBeNull();
  });
});

describe("POST /api/goals", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await post({ facultyId: faculties[0] })).statusCode).toBe(401);
  });

  it("デモアカウントなら 403 を返す（作成しない）", async () => {
    getSession.mockResolvedValue(demoSession);

    expect((await post({ facultyId: faculties[0] })).statusCode).toBe(403);
  });

  it("入力が不正なら 400 を返す（Zod）", async () => {
    expect((await post({ facultyId: "abc" })).statusCode).toBe(400);
    expect(await findFinalGoals(owner.id)).toHaveLength(0);
  });

  it("正常なら 201 で、学部と大学を付けた志望校を返す", async () => {
    const res = await post({ facultyId: faculties[0], status: "candidate" });

    expect(res.statusCode).toBe(201);
    expect(res.json()).toMatchObject({
      userId: owner.id,
      facultyId: faculties[0],
      isFirstChoice: false,
      note: null,
      status: "candidate",
      faculty: { name: "法学部", university: { type: "私立" } },
    });
    expect(await findFinalGoals(owner.id)).toHaveLength(1);
  });

  it("status を省略すると decided（受験校として確定）で登録する", async () => {
    expect((await post({ facultyId: faculties[0] })).json()).toMatchObject({ status: "decided" });
  });

  it("同じ学部の重複登録は 409 を返し、2つ目を作らない", async () => {
    await post({ facultyId: faculties[0] });

    const res = await post({ facultyId: faculties[0] });

    expect(res.statusCode).toBe(409);
    expect(await findFinalGoals(owner.id)).toHaveLength(1);
  });

  it("想定外のエラー（存在しない学部）は握りつぶさず 500 になる", async () => {
    expect((await post({ facultyId: 999999999 })).statusCode).toBe(500);
  });
});

describe("PUT /api/goals/:id", () => {
  it("他人の志望校は 404 を返し、更新しない", async () => {
    const other = await createUser();
    const id = await createFinalGoal(other.id, faculties[0]);

    const res = await put(id, { facultyId: faculties[1] });

    expect(res.statusCode).toBe(404);
    expect((await findFinalGoals(other.id))[0].facultyId).toBe(faculties[0]);
  });

  it("自分の志望校なら学部を差し替える", async () => {
    const id = await createFinalGoal(owner.id, faculties[0]);

    const res = await put(id, { facultyId: faculties[1] });

    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ id, facultyId: faculties[1] });
    expect((await findFinalGoals(owner.id))[0].facultyId).toBe(faculties[1]);
  });
});

describe("PATCH /api/goals/:id", () => {
  it("デモアカウントなら 403 を返す", async () => {
    getSession.mockResolvedValue(demoSession);

    expect((await patch(1, { note: "メモ" })).statusCode).toBe(403);
  });

  it("他人の志望校は 404 を返し、変更しない", async () => {
    const other = await createUser();
    const id = await createFinalGoal(other.id, faculties[0]);

    const res = await patch(id, { note: "メモ" });

    expect(res.statusCode).toBe(404);
    expect((await findFinalGoals(other.id))[0].note).toBeNull();
  });

  it("第一志望に立てると、それまでの第一志望が外れる", async () => {
    const before = await createFinalGoal(owner.id, faculties[0], { isFirstChoice: true });
    const after = await createFinalGoal(owner.id, faculties[1]);

    const res = await patch(after, { isFirstChoice: true });

    expect(res.statusCode).toBe(200);
    const goals = await findFinalGoals(owner.id);
    expect(goals.find((g) => g.id === before)!.isFirstChoice).toBe(false);
    expect(goals.find((g) => g.id === after)!.isFirstChoice).toBe(true);
  });

  it("付け替えの途中で失敗したら、元の第一志望が残る（第一志望0校にならない）", async () => {
    const before = await createFinalGoal(owner.id, faculties[0], { isFirstChoice: true });
    const after = await createFinalGoal(owner.id, faculties[1]);
    // 外す・立てるを両方実行したあとで失敗させ、ROLLBACK で両方とも取り消されることを確かめる
    transaction.mockImplementationOnce((fn) =>
      vi.importActual<typeof import("@/api/infra/db")>("@/api/infra/db").then((actual) =>
        actual.transaction(async (tx) => {
          await fn(tx);
          throw new Error("DB down");
        })
      )
    );

    const res = await patch(after, { isFirstChoice: true });

    expect(res.statusCode).toBe(500);
    const goals = await findFinalGoals(owner.id);
    expect(goals.find((g) => g.id === before)!.isFirstChoice).toBe(true);
    expect(goals.find((g) => g.id === after)!.isFirstChoice).toBe(false);
  });

  it("第一志望を外すだけなら、ほかの志望校には触らない", async () => {
    const id = await createFinalGoal(owner.id, faculties[0], { isFirstChoice: true });

    const res = await patch(id, { isFirstChoice: false });

    expect(res.statusCode).toBe(200);
    expect(transaction).not.toHaveBeenCalled();
    expect((await findFinalGoals(owner.id))[0].isFirstChoice).toBe(false);
  });

  it("送られてきた項目だけを更新する（未指定は触らない）", async () => {
    const id = await createFinalGoal(owner.id, faculties[0], { isFirstChoice: true, note: "前のメモ" });

    const res = await patch(id, { note: "第一志望にしたい", status: "candidate" });

    expect(res.statusCode).toBe(200);
    expect((await findFinalGoals(owner.id))[0]).toMatchObject({
      note: "第一志望にしたい",
      status: "candidate",
      isFirstChoice: true,
    });
  });

  it("メモは null で消せる", async () => {
    const id = await createFinalGoal(owner.id, faculties[0], { note: "前のメモ" });

    await patch(id, { note: null });

    expect((await findFinalGoals(owner.id))[0].note).toBeNull();
  });

  it("500文字を超えるメモは 400 を返す", async () => {
    const id = await createFinalGoal(owner.id, faculties[0]);

    const res = await patch(id, { note: "あ".repeat(501) });

    expect(res.statusCode).toBe(400);
    expect((await findFinalGoals(owner.id))[0].note).toBeNull();
  });
});

describe("DELETE /api/goals/:id", () => {
  it("未ログインなら 401 を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await del(1)).statusCode).toBe(401);
  });

  it("デモアカウントなら 403 を返す（削除しない）", async () => {
    getSession.mockResolvedValue(demoSession);

    expect((await del(1)).statusCode).toBe(403);
  });

  it("他人の志望校は 404 を返し、削除しない", async () => {
    const other = await createUser();
    const id = await createFinalGoal(other.id, faculties[0]);

    expect((await del(id)).statusCode).toBe(404);
    expect(await findFinalGoals(other.id)).toHaveLength(1);
  });

  it("自分の志望校なら削除する", async () => {
    const id = await createFinalGoal(owner.id, faculties[0]);

    const res = await del(id);

    expect(res.statusCode).toBe(200);
    expect(await findFinalGoals(owner.id)).toHaveLength(0);
  });
});
