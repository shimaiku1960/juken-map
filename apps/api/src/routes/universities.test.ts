import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerUniversityRoutes } = await import("./universities.ts");
const { buildTestApp, request } = await import("../test-support.ts");
const { cleanup, createFinalGoal, createTag, createUniversity, createUser } =
  await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerUniversityRoutes);

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("GET /api/universities", () => {
  it("未ログインなら401を返す", async () => {
    getSession.mockResolvedValue(null);

    expect((await request(app, "GET", "/api/universities")).statusCode).toBe(401);
  });

  it("大学ごとに、学部とそのタグを入れ子で返す", async () => {
    const suffix = crypto.randomUUID();
    const law = await createTag(`法-${suffix}`);
    const humanities = await createTag(`文-${suffix}`);
    const university = await createUniversity({
      faculties: [{ tagIds: [law, humanities] }, { tagIds: [] }, { tagIds: [law] }],
    });
    const empty = await createUniversity();

    const res = await request(app, "GET", "/api/universities");

    expect(res.statusCode).toBe(200);
    const body: { id: number; faculties: { tags: { name: string }[] }[] }[] = res.json();
    // 行が（大学 × 学部 × タグ）に展開されても、学部3つ・タグ 2 / 0 / 1 の入れ子に戻る
    expect(body.find((u) => u.id === university.id)?.faculties).toEqual([
      { tags: [{ name: `法-${suffix}` }, { name: `文-${suffix}` }] },
      { tags: [] },
      { tags: [{ name: `法-${suffix}` }] },
    ]);
    // 学部が1つも無い大学も、faculties: [] で含まれる
    expect(body.find((u) => u.id === empty.id)?.faculties).toEqual([]);
  });
});

describe("GET /api/universities/:id", () => {
  it("学部とタグ、登録済みの学部を返す", async () => {
    const tag = await createTag("タグ-詳細-" + crypto.randomUUID());
    const university = await createUniversity({
      name: "テスト大学-" + crypto.randomUUID(),
      faculties: [
        { name: "法学部", examDate: new Date("2027-02-15T00:00:00.000Z"), tagIds: [tag] },
        { name: "文学部" },
      ],
    });
    await createFinalGoal(owner.id, university.facultyIds[1]);

    const res = await request(app, "GET", `/api/universities/${university.id}`);

    expect(res.statusCode).toBe(200);
    const body = res.json();
    expect(body.registeredFacultyIds).toEqual([university.facultyIds[1]]);
    expect(body.university.faculties).toEqual([
      expect.objectContaining({
        id: university.facultyIds[0],
        name: "法学部",
        examDate: "2027-02-15T00:00:00.000Z",
        universityId: university.id,
        tags: [expect.objectContaining({ id: tag })],
      }),
      expect.objectContaining({ id: university.facultyIds[1], name: "文学部", tags: [] }),
    ]);
  });

  it("存在しない大学なら404を返す", async () => {
    const res = await request(app, "GET", "/api/universities/999999999");

    expect(res.statusCode).toBe(404);
  });

  it("IDが数値でなければ400を返す", async () => {
    const res = await request(app, "GET", "/api/universities/abc");

    expect(res.statusCode).toBe(400);
  });
});
