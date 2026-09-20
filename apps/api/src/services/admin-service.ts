import { execute, select, transaction } from "@/api/infra/db";
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
  bannedAt: Date | null;
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
      `SELECT u.id, u.email, u.nickname, u.name, u.kind, u.role, u.emailVerified, u.bannedAt, u.createdAt,
              (SELECT GROUP_CONCAT(DISTINCT a.providerId ORDER BY a.providerId)
                 FROM account a WHERE a.userId = u.id) AS providers,
              (SELECT MAX(s.createdAt) FROM session s WHERE s.userId = u.id) AS lastLoginAt,
              (SELECT COUNT(*) FROM StudyLog l WHERE l.userId = u.id) AS studyLogCount,
              (SELECT MAX(l.createdAt) FROM StudyLog l WHERE l.userId = u.id) AS lastStudyLogAt
       FROM (
         SELECT u.id, u.email, u.nickname, u.name, u.role, u.emailVerified, u.bannedAt, u.createdAt,
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
        bannedAt: row.bannedAt?.toISOString() ?? null,
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

// ---- 停止・削除（/admin のユーザー操作） ----
//
// 守りは3つ。自分自身・他の管理者・デモアカウントには手を出せない。
// 「自分自身」は、最後の管理者が自分を締め出して誰も入れなくなるのを防ぐため。
// 「他の管理者」は、管理者どうしで潰し合えないようにするため（付け替えは pnpm admin:grant だけ）。
// デモは面接官向けの共有アカウントで、消えると /login のデモボタンが動かなくなる。

export type ProtectedReason = "self" | "admin" | "demo";

export type UserActionOutcome<T> =
  | { result: "ok"; value: T }
  | { result: "not_found" }
  | { result: "protected"; reason: ProtectedReason }
  | { result: "email_mismatch" };

type TargetRow = {
  id: string;
  email: string | null;
  role: UserRole;
  bannedAt: Date | null;
};

/** 操作できる相手かを確かめる。できないときは理由を返す。 */
async function findTarget(
  id: string,
  actorId: string
): Promise<{ result: "ok"; value: TargetRow } | { result: "not_found" } | { result: "protected"; reason: ProtectedReason }> {
  const [user] = await select<TargetRow>(
    "SELECT id, email, role, bannedAt FROM `user` WHERE id = ?",
    [id]
  );
  if (!user) return { result: "not_found" };
  if (user.id === actorId) return { result: "protected", reason: "self" };
  if (user.role === "admin") return { result: "protected", reason: "admin" };
  if (user.email === DEMO_EMAIL) return { result: "protected", reason: "demo" };
  return { result: "ok", value: user };
}

export type BanResult = { id: string; email: string | null; bannedAt: string; sessionsRemoved: number };

/**
 * 利用者を停止する。すでに停止済みなら最初に止めた日時を保つ（押し直しても上書きしない）。
 * session を消すのは今つながっている画面をすぐ落とすため。次のログインは auth.ts の
 * session.create.before が断る。両方そろって初めて「止まった」と言える。
 */
export function banUser(id: string, actorId: string) {
  return measured("admin.banUser", async (): Promise<UserActionOutcome<BanResult>> => {
    const target = await findTarget(id, actorId);
    if (target.result !== "ok") return target;

    const now = new Date();
    return transaction(async (db) => {
      await execute(
        "UPDATE `user` SET bannedAt = COALESCE(bannedAt, ?), updatedAt = ? WHERE id = ?",
        [now, now, id],
        db
      );
      const removed = await execute("DELETE FROM session WHERE userId = ?", [id], db);
      return {
        result: "ok" as const,
        value: {
          id,
          email: target.value.email,
          bannedAt: (target.value.bannedAt ?? now).toISOString(),
          sessionsRemoved: removed.affectedRows,
        },
      };
    });
  });
}

/** 停止を解除する。止まっていなければ何も変わらない（押しても壊れない）。 */
export function unbanUser(id: string) {
  return measured("admin.unbanUser", async (): Promise<UserActionOutcome<{ id: string; email: string | null }>> => {
    const [user] = await select<TargetRow>(
      "SELECT id, email, role, bannedAt FROM `user` WHERE id = ?",
      [id]
    );
    if (!user) return { result: "not_found" };

    await execute("UPDATE `user` SET bannedAt = NULL, updatedAt = ? WHERE id = ?", [new Date(), id]);
    return { result: "ok", value: { id, email: user.email } };
  });
}

export type DeleteResult = {
  id: string;
  email: string | null;
  /** 一緒に消えた行数。外部キーの CASCADE が消すので、数えるのは記録のためだけ。 */
  removed: { studyLogs: number; studyPlans: number; textbooks: number; finalGoals: number };
};

/**
 * 利用者を消す。確認のため、呼び出し側が渡すメールアドレスが本人のものと一致しないと消さない。
 * 一覧が古いまま別の行を消してしまう事故を、id だけに頼らずここでも止める。
 *
 * ぶら下がっている行（StudyLog・StudyPlan・Textbook・FinalGoal・session・account・
 * NotificationPreference / NotificationDelivery・LINE 関連・Support 関連）は
 * 外部キーの ON DELETE CASCADE で一緒に消える。SupportCheckoutInvitation だけは
 * SET NULL なので、請求の記録は残って利用者との結び付きだけが外れる。
 */
export function deleteUser(id: string, actorId: string, email: string) {
  return measured("admin.deleteUser", async (): Promise<UserActionOutcome<DeleteResult>> => {
    const target = await findTarget(id, actorId);
    if (target.result !== "ok") return target;
    if ((target.value.email ?? "").toLowerCase() !== email.trim().toLowerCase()) {
      return { result: "email_mismatch" };
    }

    return transaction(async (db) => {
      const [counts] = await select<{
        studyLogs: number | string;
        studyPlans: number | string;
        textbooks: number | string;
        finalGoals: number | string;
      }>(
        `SELECT (SELECT COUNT(*) FROM StudyLog WHERE userId = ?) AS studyLogs,
                (SELECT COUNT(*) FROM StudyPlan WHERE userId = ?) AS studyPlans,
                (SELECT COUNT(*) FROM Textbook WHERE userId = ?) AS textbooks,
                (SELECT COUNT(*) FROM FinalGoal WHERE userId = ?) AS finalGoals`,
        [id, id, id, id],
        db
      );
      await execute("DELETE FROM `user` WHERE id = ?", [id], db);
      return {
        result: "ok" as const,
        value: {
          id,
          email: target.value.email,
          removed: {
            studyLogs: Number(counts?.studyLogs ?? 0),
            studyPlans: Number(counts?.studyPlans ?? 0),
            textbooks: Number(counts?.textbooks ?? 0),
            finalGoals: Number(counts?.finalGoals ?? 0),
          },
        },
      };
    });
  });
}
