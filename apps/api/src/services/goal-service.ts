import { execute, select, transaction, type Db } from "@/api/infra/db";
import type { FacultyRow, FinalGoalRow, TagRow, UniversityRow } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";

// 志望校 → 学部 → 大学は「多対1」の連なりなので、JOIN しても行は増えない。
// 学部 → タグだけが1対多（中間テーブル _FacultyToTag、A = Faculty.id, B = Tag.id）。
// 1対多が1本だけなので、LEFT JOIN 1本で取り、志望校ごとにタグを束ねる。
//
// Prisma の include はこれを志望校・学部・大学・タグ（中間テーブル込み）の
// 4本に分けて取っていた。

const GOAL_COLUMNS = `
  g.id, g.createdAt, g.userId, g.facultyId, g.isFirstChoice, g.note, g.status
`;

// 学部と大学の列。志望校の列名（id, createdAt）とぶつかるので f_ / u_ を付ける。
const FACULTY_COLUMNS = `
  f.id AS f_id, f.name AS f_name, f.examDate AS f_examDate,
  f.createdAt AS f_createdAt, f.universityId AS f_universityId,
  u.id AS u_id, u.name AS u_name, u.prefecture AS u_prefecture,
  u.type AS u_type, u.createdAt AS u_createdAt
`;

const FROM_GOAL_WITH_FACULTY = `
  FROM FinalGoal AS g
  JOIN Faculty AS f ON f.id = g.facultyId
  JOIN University AS u ON u.id = f.universityId
`;

type FacultyColumns = {
  f_id: number;
  f_name: string;
  f_examDate: Date;
  f_createdAt: Date;
  f_universityId: number;
  u_id: number;
  u_name: string;
  u_prefecture: string;
  u_type: string;
  u_createdAt: Date;
};

type TagColumns = {
  t_id: number | null;
  t_name: string | null;
  t_createdAt: Date | null;
};

function pickGoal(row: FinalGoalRow): FinalGoalRow {
  return {
    id: row.id,
    createdAt: row.createdAt,
    userId: row.userId,
    facultyId: row.facultyId,
    isFirstChoice: row.isFirstChoice,
    note: row.note,
    status: row.status,
  };
}

function pickFaculty(row: FacultyColumns): FacultyRow & { university: UniversityRow } {
  return {
    id: row.f_id,
    name: row.f_name,
    examDate: row.f_examDate,
    createdAt: row.f_createdAt,
    universityId: row.f_universityId,
    university: {
      id: row.u_id,
      name: row.u_name,
      prefecture: row.u_prefecture,
      type: row.u_type,
      createdAt: row.u_createdAt,
    },
  };
}

/** 志望校ページ用。学部・大学に加え、学部のタグまで引く。 */
export function listGoals(userId: string) {
  return measured("goal.list", async () => {
    // 同じ日時に作った志望校どうしの順番を決めるため、最後に id でも並べる。
    const rows = await select<FinalGoalRow & FacultyColumns & TagColumns>(
      `SELECT ${GOAL_COLUMNS}, ${FACULTY_COLUMNS},
              t.id AS t_id, t.name AS t_name, t.createdAt AS t_createdAt
       ${FROM_GOAL_WITH_FACULTY}
       LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
       LEFT JOIN Tag AS t ON t.id = ft.B
       WHERE g.userId = ?
       ORDER BY g.createdAt ASC, g.id ASC, t.id ASC`,
      [userId]
    );

    // ORDER BY で同じ志望校の行が隣り合うので、直前の要素と比べるだけで束ねられる。
    const goals: (FinalGoalRow & {
      faculty: FacultyRow & { university: UniversityRow; tags: TagRow[] };
    })[] = [];
    for (const row of rows) {
      let goal = goals.at(-1);
      if (goal?.id !== row.id) {
        goal = { ...pickGoal(row), faculty: { ...pickFaculty(row), tags: [] } };
        goals.push(goal);
      }
      // タグが1つも無い学部は、タグの列が NULL の行が1行だけ来る
      if (row.t_id !== null) {
        goal.faculty.tags.push({ id: row.t_id, name: row.t_name!, createdAt: row.t_createdAt! });
      }
    }
    return goals;
  });
}

async function findGoalWithFaculty(where: string, params: unknown[], db?: Db) {
  const [row] = await select<FinalGoalRow & FacultyColumns>(
    `SELECT ${GOAL_COLUMNS}, ${FACULTY_COLUMNS} ${FROM_GOAL_WITH_FACULTY} ${where} LIMIT 1`,
    params,
    db
  );
  return row ? { ...pickGoal(row), faculty: pickFaculty(row) } : null;
}

/** トップの「第一志望」表示専用。タグは画面で使わないので引かない。 */
export function findFirstChoiceGoal(userId: string) {
  return measured("goal.findFirstChoice", () =>
    // 第一志望は1ユーザー1校（applyGoalPatch が保つ）。DB の制約ではないので、
    // 万一2校あっても結果が揺れないよう id で並べて1件にする。
    findGoalWithFaculty(
      `WHERE g.userId = ? AND g.status = 'decided' AND g.isFirstChoice = TRUE
       ORDER BY g.id ASC`,
      [userId]
    )
  );
}

/** 志望校として登録済みの facultyId 一覧。大学詳細で「登録済み」を出し分けるのに使う。 */
export function listGoalFacultyIds(userId: string) {
  return measured("goal.listFacultyIds", () =>
    select<{ facultyId: number }>(
      "SELECT facultyId FROM FinalGoal WHERE userId = ? ORDER BY facultyId ASC",
      [userId]
    )
  );
}

// ここから下は書き込み。HTTP のことは知らないので、
// 「見つからない」は null、「一意制約違反」は ER_DUP_ENTRY の例外のまま呼び出し元へ返す。
// それを 404 や 409 にするかは routes の判断。

async function findGoalById(id: number) {
  const [row] = await select<FinalGoalRow>(
    `SELECT ${GOAL_COLUMNS} FROM FinalGoal AS g WHERE g.id = ?`,
    [id]
  );
  if (!row) throw new Error(`FinalGoal ${id} が見つかりません`);
  return row;
}

/** 更新・削除の前に「自分のものか」を確かめる。他人のものと存在しないものを区別しない。 */
export function findOwnedGoal(id: number, userId: string) {
  return measured("goal.findOwned", async () => {
    const [row] = await select<FinalGoalRow>(
      `SELECT ${GOAL_COLUMNS} FROM FinalGoal AS g WHERE g.id = ? AND g.userId = ? LIMIT 1`,
      [id, userId]
    );
    return row ?? null;
  });
}

/** 志望校を登録する。同じ学部の重複は DB の一意制約（userId, facultyId）が弾く。 */
export function createGoal(input: {
  userId: string;
  facultyId: number;
  status?: string;
}) {
  return measured("goal.create", async () => {
    const inserted = await execute(
      "INSERT INTO FinalGoal (userId, facultyId, status, createdAt) VALUES (?, ?, ?, ?)",
      [input.userId, input.facultyId, input.status ?? "decided", new Date()]
    );
    // 画面は学部名・大学名を表示するので、それも付けて返す。
    const created = await findGoalWithFaculty("WHERE g.id = ?", [inserted.insertId]);
    if (!created) throw new Error(`FinalGoal ${inserted.insertId} が見つかりません`);
    return created;
  });
}

/** 志望校の学部を差し替える。facultyId が無ければ何も変えず、今の行を返す。 */
export function updateGoal(id: number, data: { facultyId?: number }) {
  return measured("goal.update", async () => {
    if (data.facultyId) {
      await execute("UPDATE FinalGoal SET facultyId = ? WHERE id = ?", [data.facultyId, id]);
    }
    return findGoalById(id);
  });
}

/**
 * 第一志望・メモ・ステータスのうち、送られてきたものだけを更新する。
 *
 * 第一志望は1ユーザー1校までなので、付け替えは「全部外す→1件立てる」を
 * ひとつのトランザクションで行う。分けて実行すると、途中で失敗したときに
 * 第一志望が0校の状態が残る。
 */
export function applyGoalPatch(
  userId: string,
  id: number,
  patch: { isFirstChoice?: boolean; note?: string | null; status?: string }
) {
  return measured("goal.applyPatch", async () => {
    // 列名はこのコードに書いた固定の名前だけで、利用者の入力は値として ? で渡す。
    const changes: [column: string, value: unknown][] = [];
    if (patch.isFirstChoice !== undefined) changes.push(["isFirstChoice", patch.isFirstChoice]);
    if (patch.note !== undefined) changes.push(["note", patch.note]);
    if (patch.status !== undefined) changes.push(["status", patch.status]);
    if (changes.length === 0) return;

    const update = (db?: Db) =>
      execute(
        `UPDATE FinalGoal SET ${changes.map(([column]) => `${column} = ?`).join(", ")}
         WHERE id = ?`,
        [...changes.map(([, value]) => value), id],
        db
      );

    if (patch.isFirstChoice) {
      await transaction(async (tx) => {
        await execute(
          "UPDATE FinalGoal SET isFirstChoice = FALSE WHERE userId = ?",
          [userId],
          tx
        );
        await update(tx);
      });
    } else {
      // 1文だけなので、トランザクションで包まなくても途中の状態は残らない。
      await update();
    }
  });
}

/** 志望校を削除する。 */
export function deleteGoal(id: number) {
  return measured("goal.delete", async () => {
    await execute("DELETE FROM FinalGoal WHERE id = ?", [id]);
  });
}
