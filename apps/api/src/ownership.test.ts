import { afterAll, beforeAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import type { RouteOptions } from "fastify";

// 利用者の API（入口 E3）で、他人の持ち物の ID を渡すと断られることを確かめる
// （セキュリティ基準 06 の A3）。
//
// 全ルートを onRoute で集め、E3 のルートは1本残らず下の表 OWNERSHIP で分類させる。
// ルートを足して表に書かなければ落ちるので、所有者の確認を持たないルートが
// 否定テスト無しで紛れ込むことがない。path に ID を取るルートは「持ち物」か
// 「マスター」のどちらかでなければならない。
//
// 持ち物のケースは同じ形のリクエストを2回送る。ID が他人のものなら 4xx で DB が
// 変わらず、自分のものなら 2xx になる。後者が無いと、ボディの誤りで返る 400 でも
// 「断られた」ことになり、所有者の確認を外しても気づけない。

vi.mock("./auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

// microCMS は読み込むだけで API キーを要求し、呼べば外へ出ていくので差し替える。
vi.mock("@/api/infra/microcms", () => ({
  getBlog: vi.fn(),
  listBlogs: vi.fn(),
  isBlogNotFound: vi.fn(() => false),
}));

const { auth } = await import("./auth.ts");
const { registerRoutes } = await import("./routes/index.ts");
const { buildTestApp, request } = await import("./test-support.ts");
const {
  cleanup,
  createFinalGoal,
  createStudyPlan,
  createTextbook,
  createUniversity,
  createUser,
  findFinalGoals,
  findStudyLogs,
  findStudyPlan,
  findStudyPlans,
  findTextbooks,
} = await import("./test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;

type Method = Parameters<typeof request>[1];

/** 1回分のリクエストと、断られたあとに DB が変わっていないかを見るための読み取り。 */
type Attempt = {
  url: string;
  body?: unknown;
  read: () => Promise<unknown>;
};

/** caller（ログインしている人）が、holder の持ち物の ID を渡して叩くリクエストを作る。 */
type Arrange = (caller: string, holder: string) => Promise<Attempt>;

type Classification =
  /** 持ち物の ID を受け取る。ケースごとに他人の ID で叩く。 */
  | { kind: "owned"; cases: Record<string, Arrange> }
  /** 受け取る ID は全員で共有するマスター（大学・学部・参考書マスター）で、持ち主がいない。 */
  | { kind: "master"; reason: string }
  /** ID を受け取らず、セッションの人のものだけを扱う。 */
  | { kind: "none"; reason: string };

let facultyId: number;

const OWNERSHIP: Record<string, Classification> = {
  // 志望校
  "GET /api/goals": { kind: "none", reason: "自分の志望校の一覧" },
  "GET /api/goals/first-choice": { kind: "none", reason: "自分の第一志望" },
  "POST /api/goals": { kind: "master", reason: "facultyId は学部マスター" },
  "PUT /api/goals/:id": {
    kind: "owned",
    cases: {
      他人の志望校: async (_caller, holder) => {
        const id = await createFinalGoal(holder, facultyId);
        return { url: `/api/goals/${id}`, body: { status: "candidate" }, read: () => findFinalGoals(holder) };
      },
    },
  },
  "PATCH /api/goals/:id": {
    kind: "owned",
    cases: {
      他人の志望校: async (_caller, holder) => {
        const id = await createFinalGoal(holder, facultyId);
        return { url: `/api/goals/${id}`, body: { note: "書き換え" }, read: () => findFinalGoals(holder) };
      },
    },
  },
  "DELETE /api/goals/:id": {
    kind: "owned",
    cases: {
      他人の志望校: async (_caller, holder) => {
        const id = await createFinalGoal(holder, facultyId);
        return { url: `/api/goals/${id}`, read: () => findFinalGoals(holder) };
      },
    },
  },

  // 予定
  "GET /api/study-plans": { kind: "none", reason: "自分の予定の一覧" },
  "POST /api/study-plans": {
    kind: "owned",
    cases: {
      他人の参考書: async (caller, holder) => {
        const textbookId = await createTextbook(holder);
        return {
          url: "/api/study-plans",
          body: { date: "2027-02-20", items: [{ textbookId }] },
          read: () => findStudyPlans(caller),
        };
      },
    },
  },
  "PATCH /api/study-plans/:id": {
    kind: "owned",
    cases: {
      他人の予定: async (_caller, holder) => {
        const id = await createStudyPlan(holder, { content: "元の内容" });
        return { url: `/api/study-plans/${id}`, body: { content: "書き換え" }, read: () => findStudyPlan(id) };
      },
      他人の参考書: async (caller, holder) => {
        const id = await createStudyPlan(caller, { content: "元の内容" });
        const textbookId = await createTextbook(holder);
        return { url: `/api/study-plans/${id}`, body: { textbookId }, read: () => findStudyPlan(id) };
      },
    },
  },
  "DELETE /api/study-plans/:id": {
    kind: "owned",
    cases: {
      他人の予定: async (_caller, holder) => {
        const id = await createStudyPlan(holder, { content: "元の内容" });
        return { url: `/api/study-plans/${id}`, read: () => findStudyPlan(id) };
      },
    },
  },
  "POST /api/study-plans/:id/complete": {
    kind: "owned",
    cases: {
      他人の予定: async (caller, holder) => {
        const id = await createStudyPlan(holder, { content: "元の内容" });
        return {
          url: `/api/study-plans/${id}/complete`,
          body: { minutes: 30 },
          read: async () => [await findStudyPlan(id), await findStudyLogs(caller), await findStudyLogs(holder)],
        };
      },
    },
  },

  // 参考書
  "GET /api/textbooks": { kind: "none", reason: "自分の参考書の一覧" },
  "POST /api/textbooks": { kind: "master", reason: "masterId は参考書マスター（名前で作るときは ID を取らない）" },
  "PATCH /api/textbooks/:id": {
    kind: "owned",
    cases: {
      他人の参考書: async (_caller, holder) => {
        const id = await createTextbook(holder);
        return { url: `/api/textbooks/${id}`, body: { totalAmount: 100 }, read: () => findTextbooks(holder) };
      },
    },
  },
};

const userRoutes: string[] = [];
const app = buildTestApp((app) => {
  app.addHook("onRoute", (route: RouteOptions) => {
    if (route.config?.access !== "user") return;
    for (const method of [route.method].flat()) {
      if (method === "HEAD") continue;
      userRoutes.push(`${method} ${route.url}`);
    }
  });
  registerRoutes(app);
});

const ownedCases = Object.entries(OWNERSHIP).flatMap(([route, classification]) =>
  classification.kind === "owned"
    ? Object.entries(classification.cases).map(([name, arrange]) => ({ route, name, arrange }))
    : []
);

beforeAll(async () => {
  await app.ready();
  const university = await createUniversity({ faculties: [{}] });
  facultyId = university.facultyIds[0];
});

beforeEach(() => {
  getSession.mockReset();
});

afterAll(cleanup);

describe("A3 表と登録されたルートの突き合わせ", () => {
  it("利用者の API は全件が表で分類されていて、表に余りも無い", () => {
    // 0件のまま通る（何も突き合わせていない）状態を防ぐ。
    expect(userRoutes.length).toBeGreaterThan(10);
    expect(userRoutes.filter((route) => !(route in OWNERSHIP))).toEqual([]);
    expect(Object.keys(OWNERSHIP).filter((route) => !userRoutes.includes(route))).toEqual([]);
  });

  it("path に ID を取るルートは「持ち物」か「マスター」に分類されている", () => {
    const withParam = userRoutes.filter((route) => route.includes("/:"));
    expect(withParam.length).toBeGreaterThan(0);
    expect(withParam.filter((route) => OWNERSHIP[route].kind === "none")).toEqual([]);
  });
});

describe("A3 他人の ID", () => {
  it.each(ownedCases)("$route：$name なら断り、DB を変えない", async ({ route, arrange }) => {
    const method = route.split(" ")[0] as Method;
    const caller = await createUser();
    const holder = await createUser();
    getSession.mockResolvedValue(caller.session);

    const attempt = await arrange(caller.id, holder.id);
    const before = await attempt.read();
    const res = await request(app, method, attempt.url, attempt.body);

    expect([400, 403, 404]).toContain(res.statusCode);
    expect(await attempt.read()).toEqual(before);
  });

  it.each(ownedCases)("$route：$name を自分の ID に替えると通る", async ({ route, arrange }) => {
    const method = route.split(" ")[0] as Method;
    const caller = await createUser();
    getSession.mockResolvedValue(caller.session);

    const attempt = await arrange(caller.id, caller.id);
    const res = await request(app, method, attempt.url, attempt.body);

    expect(res.statusCode, res.body).toBeGreaterThanOrEqual(200);
    expect(res.statusCode, res.body).toBeLessThan(300);
  });
});
