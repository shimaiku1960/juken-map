import zlib from "node:zlib";
import fastifyCompress from "@fastify/compress";
import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const { auth } = await import("../auth.ts");
const { pickEncoding, registerUniversityRoutes } = await import("./universities.ts");
const { buildTestApp, request } = await import("../test-support.ts");
const { cleanup, createFinalGoal, createTag, createUniversity, createUser, trackUniversity } =
  await import("../test-db/fixtures.ts");
const { invalidateUniversitiesForExplore } = await import("../services/university-service.ts");
const masterService = await import("../services/master-service.ts");

const getSession = auth.api.getSession as unknown as Mock;
const app = buildTestApp(registerUniversityRoutes);

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
  // fixtures は管理画面を通さず DB に直接入れるので、一覧のキャッシュは自分で捨てる。
  invalidateUniversitiesForExplore();
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

  it("ETag を付けて返し、同じ ETag で聞かれたら本文なしの304を返す", async () => {
    const first = await request(app, "GET", "/api/universities");
    const etag = first.headers.etag as string;
    expect(first.headers["cache-control"]).toBe("private, no-cache");
    expect(etag).toMatch(/^"[\w-]+"$/);

    const notModified = await request(app, "GET", "/api/universities", undefined, {
      "if-none-match": etag,
    });
    expect(notModified.statusCode).toBe(304);
    expect(notModified.body).toBe("");
    expect(notModified.headers.etag).toBe(etag);

    // 途中で圧縮し直されて弱い ETag になっても、複数並んでいても一致とみなす
    const weak = await request(app, "GET", "/api/universities", undefined, {
      "if-none-match": `"other", W/${etag}`,
    });
    expect(weak.statusCode).toBe(304);

    const stale = await request(app, "GET", "/api/universities", undefined, {
      "if-none-match": '"other"',
    });
    expect(stale.statusCode).toBe(200);
  });

  it("DB を直接変えても、キャッシュの間は同じ一覧を返す", async () => {
    const before = await request(app, "GET", "/api/universities");
    const added = await createUniversity();

    const cached = await request(app, "GET", "/api/universities");
    expect(cached.headers.etag).toBe(before.headers.etag);
    expect(cached.json().some((u: { id: number }) => u.id === added.id)).toBe(false);
  });

  it("管理画面で大学・学部を変えたら、次の一覧に反映される", async () => {
    const first = await request(app, "GET", "/api/universities");

    const created = await masterService.createUniversity({
      name: `管理で追加-${crypto.randomUUID()}`,
      prefecture: "東京都",
      type: "私立",
    });
    if (created.result !== "ok") throw new Error("大学を作れなかった");
    trackUniversity(created.value.id);

    const afterUniversity = await request(app, "GET", "/api/universities");
    expect(afterUniversity.headers.etag).not.toBe(first.headers.etag);
    expect(afterUniversity.json().some((u: { id: number }) => u.id === created.value.id)).toBe(true);

    // 学部はトランザクションの中で作るので、確定後に捨てていることも確かめる
    const faculty = await masterService.createFaculty({
      universityId: created.value.id,
      name: "法学部",
      examDate: "2027-02-15",
      tagIds: [],
    });
    expect(faculty.result).toBe("ok");

    const afterFaculty = await request(app, "GET", "/api/universities");
    const university = afterFaculty
      .json()
      .find((u: { id: number }) => u.id === created.value.id);
    expect(university.faculties).toEqual([{ tags: [] }]);
  });
});

describe("GET /api/universities の圧縮", () => {
  // 本番（server.ts）と同じ設定の @fastify/compress を載せたアプリ。
  // ルートを後から読み込む plugin にして、圧縮のフックが確実にかかった状態で試す。
  const compressedApp = buildTestApp((app) => {
    app.register(fastifyCompress, { global: true, encodings: ["br", "gzip", "deflate"] });
    app.register(async (instance) => registerUniversityRoutes(instance));
  });

  it("br を受け付けるなら、キャッシュした Brotli 版を返し、展開すると圧縮なしの本文と一致する", async () => {
    const plain = await request(compressedApp, "GET", "/api/universities");
    const res = await request(compressedApp, "GET", "/api/universities", undefined, {
      "accept-encoding": "gzip, deflate, br, zstd",
    });

    expect(res.statusCode).toBe(200);
    expect(res.headers["content-encoding"]).toBe("br");
    expect(res.headers.vary).toMatch(/accept-encoding/i);
    // 二重に圧縮されていれば、1回展開しただけでは JSON に戻らない
    expect(zlib.brotliDecompressSync(res.rawPayload).toString()).toBe(plain.body);
    expect(plain.headers["content-encoding"]).toBeUndefined();
  });

  it("br を受け付けず gzip なら gzip 版を返す", async () => {
    const plain = await request(compressedApp, "GET", "/api/universities");
    for (const acceptEncoding of ["gzip", "br;q=0, gzip"]) {
      const res = await request(compressedApp, "GET", "/api/universities", undefined, {
        "accept-encoding": acceptEncoding,
      });

      expect(res.headers["content-encoding"]).toBe("gzip");
      expect(zlib.gunzipSync(res.rawPayload).toString()).toBe(plain.body);
    }
  });

  it("圧縮しても ETag は同じで、304 も返る", async () => {
    const plain = await request(compressedApp, "GET", "/api/universities");
    const res = await request(compressedApp, "GET", "/api/universities", undefined, {
      "accept-encoding": "br",
      "if-none-match": plain.headers.etag as string,
    });

    expect(res.statusCode).toBe(304);
  });
});

describe("pickEncoding", () => {
  it("br を優先し、q=0 は受け付けないものとして扱う", () => {
    expect(pickEncoding("gzip, deflate, br, zstd")).toBe("br");
    expect(pickEncoding("gzip;q=1.0, br;q=0.5")).toBe("br");
    expect(pickEncoding("br;q=0, gzip")).toBe("gzip");
    expect(pickEncoding("BR")).toBe("br");
    expect(pickEncoding("*")).toBe("br");
    expect(pickEncoding("deflate")).toBeNull();
    expect(pickEncoding("identity")).toBeNull();
    expect(pickEncoding("")).toBeNull();
    expect(pickEncoding(undefined)).toBeNull();
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
