import { afterAll, beforeAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import type { RouteOptions } from "fastify";

// 利用者が変えてはいけない項目（ロール・持ち主・停止状態・メール確認状態など）を、
// 書き込みの本文に混ぜても書き換わらないことを確かめる（セキュリティ基準 06 の A4）。
//
// 利用者の API（入口 E3）の書き込みを onRoute で全件集め、下の表 WRITES に1本ずつ
// 「成功する本文」を書かせる。表に無いルートがあれば落ちるので、ルートを足して
// 確かめ忘れることがない。各ルートに、成功する本文＋禁止項目を送り、
//   - 2xx で通る（禁止項目のせいで 400 になっただけなら、何も確かめていない）
//   - 呼んだ人の role・bannedAt・emailVerified・email が変わらない
//   - 別の人の行が1つも増えず変わらない（userId に別の人を入れて付け替える攻撃）
//   - 呼んだ人の行の id が、送った id になっていない
// を見る。Better Auth の登録・更新は auth.forbidden-fields.test.ts で見る。

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
const { select } = await import("@/api/infra/db");
const { todayYmdTokyo } = await import("@/shared/date");
const {
  cleanup,
  createFinalGoal,
  createLineConnection,
  createStudyLog,
  createStudyPlan,
  createTextbook,
  createUniversity,
  createUser,
} = await import("./test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;

type Method = Parameters<typeof request>[1];

/** どのテーブルにも無い大きさの id。これが行の id になっていたら、本文の id が使われた。 */
const FORBIDDEN_ID = 2_000_000_000;

/** 本文に混ぜる禁止項目。userId には別の人（victim）を入れる。 */
function forbiddenFields(victimId: string) {
  return {
    id: FORBIDDEN_ID,
    userId: victimId,
    role: "admin",
    bannedAt: "2000-01-01T00:00:00.000Z",
    emailVerified: true,
    email: "taken-over@example.test",
  };
}

type Forbidden = ReturnType<typeof forbiddenFields>;

/**
 * caller が成功するはずの書き込みを作る。本文の最上位には呼び出し側で禁止項目を足す。
 * 配列の中の要素にも項目を持つ本文（予定の作成）は、ここで forbidden を混ぜる。
 */
type Arrange = (caller: string, forbidden: Forbidden) => Promise<{ url: string; body?: object }>;

let facultyId: number;

const WRITES: Record<string, Arrange> = {
  // 志望校
  "POST /api/goals": async () => ({ url: "/api/goals", body: { facultyId } }),
  "PUT /api/goals/:id": async (caller) => {
    const id = await createFinalGoal(caller, facultyId);
    return { url: `/api/goals/${id}`, body: { status: "candidate" } };
  },
  "PATCH /api/goals/:id": async (caller) => {
    const id = await createFinalGoal(caller, facultyId);
    return { url: `/api/goals/${id}`, body: { note: "メモ" } };
  },
  "DELETE /api/goals/:id": async (caller) => {
    const id = await createFinalGoal(caller, facultyId);
    return { url: `/api/goals/${id}` };
  },

  // 実績
  "POST /api/study-logs": async () => ({
    url: "/api/study-logs",
    body: { date: todayYmdTokyo(), minutes: 30 },
  }),
  "PATCH /api/study-logs/:id": async (caller) => {
    const id = await createStudyLog(caller);
    return { url: `/api/study-logs/${id}`, body: { date: todayYmdTokyo(), minutes: 45 } };
  },
  "DELETE /api/study-logs/:id": async (caller) => {
    const id = await createStudyLog(caller);
    return { url: `/api/study-logs/${id}` };
  },

  // 予定
  "POST /api/study-plans": async (_caller, forbidden) => ({
    url: "/api/study-plans",
    body: { date: "2027-02-20", items: [{ content: "予定", ...forbidden }] },
  }),
  "PATCH /api/study-plans/:id": async (caller) => {
    const id = await createStudyPlan(caller, { content: "元の内容" });
    return { url: `/api/study-plans/${id}`, body: { content: "書き換え" } };
  },
  "DELETE /api/study-plans/:id": async (caller) => {
    const id = await createStudyPlan(caller, { content: "元の内容" });
    return { url: `/api/study-plans/${id}` };
  },
  "POST /api/study-plans/:id/complete": async (caller) => {
    const id = await createStudyPlan(caller, { content: "元の内容" });
    return { url: `/api/study-plans/${id}/complete`, body: { minutes: 30 } };
  },

  // 参考書
  "POST /api/textbooks": async () => ({ url: "/api/textbooks", body: { name: "参考書" } }),
  "PATCH /api/textbooks/:id": async (caller) => {
    const id = await createTextbook(caller);
    return { url: `/api/textbooks/${id}`, body: { totalAmount: 100 } };
  },

  // 設定・連携
  "PUT /api/profile": async () => ({ url: "/api/profile", body: { nickname: "なまえ" } }),
  "PUT /api/notification-preferences": async () => ({
    url: "/api/notification-preferences",
    body: {
      emailMorningEnabled: true,
      emailEveningEnabled: false,
      lineMorningEnabled: false,
      lineEveningEnabled: false,
    },
  }),
  "POST /api/line/account-link": async () => ({
    url: "/api/line/account-link",
    body: { linkToken: "link-token" },
  }),
  "DELETE /api/line/connection": async (caller) => {
    await createLineConnection(caller);
    return { url: "/api/line/connection" };
  },
  "POST /api/analytics/registration": async () => ({ url: "/api/analytics/registration" }),
};

/** 呼んだ人の、利用者が変えてはいけない列。 */
async function protectedColumnsOf(userId: string) {
  const [row] = await select<{ role: string; bannedAt: Date | null; emailVerified: boolean; email: string }>(
    "SELECT role, bannedAt, emailVerified, email FROM `user` WHERE id = ?",
    [userId]
  );
  return row;
}

let tablesWithUserId: string[] = [];

/**
 * その人の行を、userId を持つ全テーブルから集める。テーブルは information_schema から
 * 引くので、あとからテーブルを足しても自動で対象に入る。並び順に頼らないよう文字列にして並べる。
 */
async function rowsOf(userId: string) {
  const rows: Record<string, string[]> = {
    user: (await select("SELECT * FROM `user` WHERE id = ?", [userId])).map((row) => JSON.stringify(row)),
  };
  for (const table of tablesWithUserId) {
    const found = await select<Record<string, unknown>>(`SELECT * FROM \`${table}\` WHERE userId = ?`, [userId]);
    rows[table] = found.map((row) => JSON.stringify(row)).sort();
  }
  return rows;
}

function idsIn(rows: Record<string, string[]>) {
  return Object.values(rows)
    .flat()
    .map((row) => (JSON.parse(row) as { id?: unknown }).id);
}

const userWrites: string[] = [];
const app = buildTestApp((app) => {
  app.addHook("onRoute", (route: RouteOptions) => {
    if (route.config?.access !== "user") return;
    for (const method of [route.method].flat()) {
      if (method === "GET" || method === "HEAD") continue;
      userWrites.push(`${method} ${route.url}`);
    }
  });
  registerRoutes(app);
});

beforeAll(async () => {
  await app.ready();
  const university = await createUniversity({ faculties: [{}] });
  facultyId = university.facultyIds[0];
  const tables = await select<{ name: string }>(
    `SELECT TABLE_NAME AS name FROM information_schema.COLUMNS
     WHERE TABLE_SCHEMA = DATABASE() AND COLUMN_NAME = 'userId' ORDER BY TABLE_NAME`
  );
  tablesWithUserId = tables.map((table) => table.name);
});

beforeEach(() => {
  getSession.mockReset();
});

afterAll(cleanup);

describe("A4 表と登録されたルートの突き合わせ", () => {
  it("利用者の API の書き込みは全件が表にあり、表に余りも無い", () => {
    // 0件のまま通る（何も突き合わせていない）状態を防ぐ。
    expect(userWrites.length).toBeGreaterThan(15);
    expect(userWrites.filter((route) => !(route in WRITES))).toEqual([]);
    expect(Object.keys(WRITES).filter((route) => !userWrites.includes(route))).toEqual([]);
  });

  it("持ち主の列を持つテーブルを DB から拾えている", () => {
    expect(tablesWithUserId).toEqual(expect.arrayContaining(["FinalGoal", "StudyLog", "StudyPlan", "Textbook"]));
  });
});

describe("A4 禁止項目", () => {
  it.each(Object.entries(WRITES))("%s：禁止項目を混ぜても通り、書き換わらない", async (route, arrange) => {
    const method = route.split(" ")[0] as Method;
    const caller = await createUser();
    const victim = await createUser();
    getSession.mockResolvedValue(caller.session);

    const forbidden = forbiddenFields(victim.id);
    const { url, body } = await arrange(caller.id, forbidden);
    const callerBefore = await protectedColumnsOf(caller.id);
    const victimBefore = await rowsOf(victim.id);

    const res = await request(app, method, url, { ...body, ...forbidden });

    expect(res.statusCode, res.body).toBeGreaterThanOrEqual(200);
    expect(res.statusCode, res.body).toBeLessThan(300);
    expect(await protectedColumnsOf(caller.id)).toEqual(callerBefore);
    expect(await rowsOf(victim.id)).toEqual(victimBefore);
    expect(idsIn(await rowsOf(caller.id))).not.toContain(FORBIDDEN_ID);
  });
});
