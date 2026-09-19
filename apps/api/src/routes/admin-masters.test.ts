import { randomUUID } from "node:crypto";
import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { registerAdminMasterRoutes } = await import("./admin-masters.ts");
const { buildTestApp, request, loggedInSession, takeLogLines } = await import("../test-support.ts");
const { select } = await import("@/api/infra/db");
const { cleanup, createFinalGoal, createTag, createUniversity, createUser, trackUniversity } =
  await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerAdminMasterRoutes);
const adminSession = { user: { id: "admin-1", email: "admin@example.com", role: "admin" } };

const universityBody = (name = `大学-${randomUUID()}`) => ({ name, prefecture: "東京都", type: "私立" });

beforeEach(() => {
  vi.clearAllMocks();
  getSession.mockResolvedValue(adminSession);
  takeLogLines();
});

afterAll(cleanup);

describe("管理者ガード", () => {
  const cases: [string, string][] = [
    ["GET", "/api/admin/universities"],
    ["GET", "/api/admin/universities/1"],
    ["GET", "/api/admin/tags"],
    ["POST", "/api/admin/universities"],
    ["PATCH", "/api/admin/universities/1"],
    ["DELETE", "/api/admin/universities/1"],
    ["POST", "/api/admin/faculties"],
    ["PATCH", "/api/admin/faculties/1"],
    ["DELETE", "/api/admin/faculties/1"],
  ];

  it.each(cases)("%s %s は未ログインなら401", async (method, url) => {
    getSession.mockResolvedValue(null);
    expect((await request(app, method as "GET", url, {})).statusCode).toBe(401);
  });

  it.each(cases)("%s %s は admin でなければ403", async (method, url) => {
    getSession.mockResolvedValue({ user: { ...loggedInSession.user, role: "user" } });
    expect((await request(app, method as "GET", url, {})).statusCode).toBe(403);
  });
});

describe("大学", () => {
  it("作成・検索・編集でき、変更がログに残る", async () => {
    const name = `大学-${randomUUID()}`;
    const created = await request(app, "POST", "/api/admin/universities", universityBody(name));
    expect(created.statusCode).toBe(201);
    const { id } = created.json() as { id: number };
    trackUniversity(id);

    const list = await request(app, "GET", `/api/admin/universities?q=${encodeURIComponent(name)}`);
    expect(list.json()).toMatchObject({ total: 1, universities: [{ id, name, facultyCount: 0, goalCount: 0 }] });

    const updated = await request(app, "PATCH", `/api/admin/universities/${id}`, {
      name,
      prefecture: "大阪府",
      type: "公立",
    });
    expect(updated.statusCode).toBe(200);
    expect(updated.json()).toMatchObject({ prefecture: "大阪府", type: "公立" });

    const logs = takeLogLines().filter((line) => line.msg === "admin master change");
    expect(logs.map((line) => line.action)).toEqual(["create", "update"]);
    expect(logs[1]).toMatchObject({
      adminId: "admin-1",
      table: "University",
      before: { prefecture: "東京都" },
      after: { prefecture: "大阪府" },
    });
  });

  it("検索の % や _ は文字として扱う", async () => {
    const list = await request(app, "GET", "/api/admin/universities?q=%25%25%25");
    expect(list.json()).toMatchObject({ total: 0 });
  });

  it("同じ名前は409、都道府県・種別が不正なら400", async () => {
    const { id } = await createUniversity();
    const [existing] = await select<{ name: string }>("SELECT name FROM University WHERE id = ?", [id]);

    expect((await request(app, "POST", "/api/admin/universities", universityBody(existing!.name))).statusCode).toBe(409);
    expect(
      (await request(app, "POST", "/api/admin/universities", { ...universityBody(), prefecture: "東京" })).statusCode
    ).toBe(400);
    expect(
      (await request(app, "POST", "/api/admin/universities", { ...universityBody(), type: "私学" })).statusCode
    ).toBe(400);
  });

  it("配下の学部が志望校に使われていれば削除できず、使われていなければ学部ごと消える", async () => {
    const used = await createUniversity({ faculties: [{ name: "使われている学部" }] });
    const user = await createUser();
    await createFinalGoal(user.id, used.facultyIds[0]!);

    const blocked = await request(app, "DELETE", `/api/admin/universities/${used.id}`);
    expect(blocked.statusCode).toBe(409);
    expect(blocked.json()).toEqual({ error: "この大学の学部が志望校に1件使われているため削除できません" });

    const unused = await createUniversity({ faculties: [{ name: "学部" }] });
    expect((await request(app, "DELETE", `/api/admin/universities/${unused.id}`)).statusCode).toBe(204);
    expect(await select("SELECT id FROM Faculty WHERE universityId = ?", [unused.id])).toEqual([]);
  });

  it("無い大学は404", async () => {
    expect((await request(app, "GET", "/api/admin/universities/999999999")).statusCode).toBe(404);
    expect((await request(app, "PATCH", "/api/admin/universities/999999999", universityBody())).statusCode).toBe(404);
  });
});

describe("学部", () => {
  it("タグ付きで作成・編集でき、詳細に受験日（日付）とタグが出る", async () => {
    const { id: universityId } = await createUniversity();
    const [tagA, tagB] = [await createTag(), await createTag()];

    const created = await request(app, "POST", "/api/admin/faculties", {
      universityId,
      name: "新しい学部",
      examDate: "2027-02-20",
      tagIds: [tagA],
    });
    expect(created.statusCode).toBe(201);
    const { id } = created.json() as { id: number };

    const updated = await request(app, "PATCH", `/api/admin/faculties/${id}`, {
      name: "名前を変えた学部",
      examDate: "2027-02-21",
      tagIds: [tagB, tagA],
    });
    expect(updated.statusCode).toBe(200);

    const detail = await request(app, "GET", `/api/admin/universities/${universityId}`);
    expect(detail.json()).toMatchObject({
      university: { id: universityId, facultyCount: 1 },
      faculties: [{ id, name: "名前を変えた学部", examDate: "2027-02-21", goalCount: 0 }],
    });
    const tagIds = (detail.json() as { faculties: { tags: { id: number }[] }[] }).faculties[0]!.tags.map((t) => t.id);
    expect(tagIds.toSorted()).toEqual([tagA, tagB].toSorted());

    // 受験日は seed と同じく UTC の0時で保存する（既存の表示の日付をずらさない）。
    const [row] = await select<{ examDate: Date }>("SELECT examDate FROM Faculty WHERE id = ?", [id]);
    expect(row!.examDate.toISOString()).toBe("2027-02-21T00:00:00.000Z");
  });

  it("同じ大学に同じ名前は409、存在しないタグ・大学は400/404", async () => {
    const { id: universityId } = await createUniversity({ faculties: [{ name: "法学部" }] });
    const body = { universityId, name: "法学部", examDate: "2027-02-15", tagIds: [] };

    expect((await request(app, "POST", "/api/admin/faculties", body)).statusCode).toBe(409);
    expect(
      (await request(app, "POST", "/api/admin/faculties", { ...body, name: "別の学部", tagIds: [999999999] })).statusCode
    ).toBe(400);
    expect(
      (await request(app, "POST", "/api/admin/faculties", { ...body, universityId: 999999999 })).statusCode
    ).toBe(404);
    expect(
      (await request(app, "POST", "/api/admin/faculties", { ...body, name: "別", examDate: "2027/02/15" })).statusCode
    ).toBe(400);
  });

  it("志望校に使われている学部は削除できず、使われていなければ消せる", async () => {
    const { facultyIds } = await createUniversity({ faculties: [{ name: "A" }, { name: "B" }] });
    const user = await createUser();
    await createFinalGoal(user.id, facultyIds[0]!);

    const blocked = await request(app, "DELETE", `/api/admin/faculties/${facultyIds[0]}`);
    expect(blocked.statusCode).toBe(409);
    expect(blocked.json()).toEqual({ error: "この学部が志望校に1件使われているため削除できません" });

    expect((await request(app, "DELETE", `/api/admin/faculties/${facultyIds[1]}`)).statusCode).toBe(204);
    expect(await select("SELECT id FROM Faculty WHERE id = ?", [facultyIds[1]])).toEqual([]);
  });
});
