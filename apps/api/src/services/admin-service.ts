import { select } from "@/api/infra/db";
import type { UserRole } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";
import { DEMO_EMAIL } from "@/shared/demo";
import {
  USER_KINDS,
  type AdminOverview,
  type AdminUserList,
  type KindStats,
  type UserKind,
} from "@/shared/dto/admin";
import { SEED_EMAIL_LIKE } from "@/shared/synthetic";

// 管理者ページ（/admin）の集計と一覧。読み取りだけ。
//
// 合成ユーザーが実ユーザーの数字に混ざらないよう、種別を SQL の CASE で決める。
// 判定の順番に意味がある：sim は simSeq で、seed はメールの印で、デモは固定アドレスで見分ける。
// 「？」を含むので、使うたびに KIND_PARAMS を同じ位置に並べる。
const KIND_SQL = `CASE
  WHEN u.simSeq IS NOT NULL THEN 'sim'
  WHEN u.email LIKE ? THEN 'seed'
  WHEN u.email = ? THEN 'demo'
  ELSE 'real'
END`;
const KIND_PARAMS = [SEED_EMAIL_LIKE, DEMO_EMAIL];

export const ADMIN_USERS_PAGE_SIZE = 50;

const DAY_MS = 24 * 60 * 60 * 1000;

type KindStatsRow = {
  kind: UserKind;
  total: number;
  // SUM() は DECIMAL で返り、mysql2 は文字列にする。数値へは呼び出し側で直す。
  verified: string | null;
  newLast7Days: string | null;
  activeLast7Days: string | null;
  activeLast30Days: string | null;
};

/** 種別ごとの人数と、実ユーザーの日別新規登録。 */
export function getAdminOverview(now = new Date()) {
  return measured("admin.overview", async (): Promise<AdminOverview> => {
    const since7 = new Date(now.getTime() - 7 * DAY_MS);
    const since30 = new Date(now.getTime() - 30 * DAY_MS);

    // 「記録した人」は StudyLog を作った日時（createdAt）で数える。学習した日（date）は
    // 「あとから記録」で過去日にもなるので、来訪の指標には使わない。
    // EXISTS は StudyLog の userId 索引で1人ずつ引くので、全件を集計しない。
    const rows = await select<KindStatsRow>(
      `SELECT ${KIND_SQL} AS kind,
              COUNT(*) AS total,
              SUM(u.emailVerified) AS verified,
              SUM(u.createdAt >= ?) AS newLast7Days,
              SUM(EXISTS (SELECT 1 FROM StudyLog l WHERE l.userId = u.id AND l.createdAt >= ?)) AS activeLast7Days,
              SUM(EXISTS (SELECT 1 FROM StudyLog l WHERE l.userId = u.id AND l.createdAt >= ?)) AS activeLast30Days
       FROM \`user\` u
       GROUP BY kind`,
      [...KIND_PARAMS, since7, since7, since30]
    );

    // 1人もいない種別も 0 で並べ、画面の並びを毎回同じにする。
    const kinds: KindStats[] = USER_KINDS.map((kind) => {
      const row = rows.find((candidate) => candidate.kind === kind);
      return {
        kind,
        total: Number(row?.total ?? 0),
        verified: Number(row?.verified ?? 0),
        newLast7Days: Number(row?.newLast7Days ?? 0),
        activeLast7Days: Number(row?.activeLast7Days ?? 0),
        activeLast30Days: Number(row?.activeLast30Days ?? 0),
      };
    });

    // 日付は利用者の感覚に合わせて Asia/Tokyo で区切る。DATETIME は UTC で保存している（db.ts）。
    // CONVERT_TZ に地域名ではなく時差を渡すのは、MySQL のタイムゾーン表が無くても動くようにするため。
    const signups = await select<{ date: string; count: number }>(
      `SELECT DATE_FORMAT(CONVERT_TZ(u.createdAt, '+00:00', '+09:00'), '%Y-%m-%d') AS date,
              COUNT(*) AS count
       FROM \`user\` u
       WHERE ${KIND_SQL} = 'real' AND u.createdAt >= ?
       GROUP BY date
       ORDER BY date`,
      [...KIND_PARAMS, since30]
    );

    return {
      kinds,
      realSignupsByDay: signups.map((row) => ({ date: row.date, count: Number(row.count) })),
    };
  });
}

type AdminUserRow = {
  id: string;
  email: string | null;
  nickname: string | null;
  name: string | null;
  kind: UserKind;
  role: UserRole;
  emailVerified: boolean;
  createdAt: Date;
  providers: string | null;
  lastLoginAt: Date | null;
  studyLogCount: number;
  lastStudyLogAt: Date | null;
};

// LIKE の % と _ を文字として扱う（検索語に含まれても全件一致にならないように）。
function escapeLike(value: string) {
  return value.replaceAll(/[\\%_]/g, (char) => `\\${char}`);
}

/** 種別で絞ったユーザー一覧（新しい順、1ページ50人）。q はメールアドレスの部分一致。 */
export function listAdminUsers(params: { kind: UserKind; q?: string; page: number }) {
  return measured("admin.listUsers", async (): Promise<AdminUserList> => {
    const where = [`${KIND_SQL} = ?`];
    const whereParams: unknown[] = [...KIND_PARAMS, params.kind];
    const q = params.q?.trim();
    if (q) {
      where.push("u.email LIKE ?");
      whereParams.push(`%${escapeLike(q)}%`);
    }
    const whereSql = where.join(" AND ");

    const [{ total }] = await select<{ total: number }>(
      `SELECT COUNT(*) AS total FROM \`user\` u WHERE ${whereSql}`,
      whereParams
    );

    // 先に50人だけを切り出し（内側）、その50人にだけ件数や最終ログインを数える（外側）。
    // 内側に LIMIT があるので MySQL は派生表を外へ展開せず、集計は50人分で済む。
    // 手元の合成データ（2000人・StudyLog 16.7万件）でも全件を数えない。
    const rows = await select<AdminUserRow>(
      `SELECT u.id, u.email, u.nickname, u.name, u.kind, u.role, u.emailVerified, u.createdAt,
              (SELECT GROUP_CONCAT(DISTINCT a.providerId ORDER BY a.providerId)
                 FROM account a WHERE a.userId = u.id) AS providers,
              (SELECT MAX(s.createdAt) FROM session s WHERE s.userId = u.id) AS lastLoginAt,
              (SELECT COUNT(*) FROM StudyLog l WHERE l.userId = u.id) AS studyLogCount,
              (SELECT MAX(l.createdAt) FROM StudyLog l WHERE l.userId = u.id) AS lastStudyLogAt
       FROM (
         SELECT u.id, u.email, u.nickname, u.name, u.role, u.emailVerified, u.createdAt,
                ${KIND_SQL} AS kind
         FROM \`user\` u
         WHERE ${whereSql}
         ORDER BY u.createdAt DESC, u.id
         LIMIT ? OFFSET ?
       ) u
       ORDER BY u.createdAt DESC, u.id`,
      [
        ...KIND_PARAMS,
        ...whereParams,
        ADMIN_USERS_PAGE_SIZE,
        (params.page - 1) * ADMIN_USERS_PAGE_SIZE,
      ]
    );

    return {
      users: rows.map((row) => ({
        id: row.id,
        email: row.email,
        nickname: row.nickname,
        name: row.name,
        kind: row.kind,
        role: row.role,
        emailVerified: row.emailVerified,
        createdAt: row.createdAt.toISOString(),
        providers: row.providers ? row.providers.split(",") : [],
        lastLoginAt: row.lastLoginAt?.toISOString() ?? null,
        studyLogCount: Number(row.studyLogCount),
        lastStudyLogAt: row.lastStudyLogAt?.toISOString() ?? null,
      })),
      total: Number(total),
      page: params.page,
      pageSize: ADMIN_USERS_PAGE_SIZE,
    };
  });
}
