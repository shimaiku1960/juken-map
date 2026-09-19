import type { PoolConnection } from "mysql2/promise";
import { execute, isDuplicateEntry, select, transaction } from "@/api/infra/db";
import type { FacultyRow, TagRow, UniversityRow } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";
import type {
  AdminFaculty,
  AdminTag,
  AdminUniversity,
  AdminUniversityDetail,
  AdminUniversityList,
} from "@/shared/dto/admin";
import type { CreateFacultyInput, FacultyInput, UniversityInput } from "@/shared/validations/master";

// 管理者ページのマスター編集（大学・学部）。
//
// 削除は「誰にも使われていない行」に限る。
//   - 学部：志望校（FinalGoal）が参照していれば消せない（DB も ON DELETE RESTRICT で拒む）
//   - 大学：学部は CASCADE で一緒に消えるので、配下の学部がどれか志望校に使われていれば止める
// 事前に数えて断るのが基本で、数えたあとに志望校が増えた場合も、DB の外部キーが拒んだエラーを
// 同じ「使われている」に読み替える（二重の守り）。

export const ADMIN_UNIVERSITIES_PAGE_SIZE = 50;

type Outcome<T> =
  | { result: "ok"; value: T }
  | { result: "not_found" }
  | { result: "duplicate" }
  | { result: "in_use"; goalCount: number }
  | { result: "invalid_tags" };

// COUNT / SUM は BIGINT・DECIMAL で返り、mysql2 は文字列にすることがあるので数値に直す。
type CountRow = { facultyCount: number | string; goalCount: number | string };

function toAdminUniversity(row: Pick<UniversityRow, "id" | "name" | "prefecture" | "type"> & CountRow): AdminUniversity {
  return {
    id: row.id,
    name: row.name,
    prefecture: row.prefecture,
    type: row.type,
    facultyCount: Number(row.facultyCount),
    goalCount: Number(row.goalCount),
  };
}

// 外部キーに参照されていて消せない（1451 ER_ROW_IS_REFERENCED_2）。
function isReferenced(error: unknown) {
  return typeof error === "object" && error !== null && "code" in error && error.code === "ER_ROW_IS_REFERENCED_2";
}

// LIKE の % と _ を文字として扱う。
function likePattern(q: string) {
  return `%${q.replace(/[\\%_]/g, (c) => `\\${c}`)}%`;
}

const UNIVERSITY_COLUMNS = `u.id, u.name, u.prefecture, u.type,
  (SELECT COUNT(*) FROM Faculty f WHERE f.universityId = u.id) AS facultyCount,
  (SELECT COUNT(*) FROM FinalGoal g JOIN Faculty f ON f.id = g.facultyId WHERE f.universityId = u.id) AS goalCount`;

export function listAdminUniversities(params: { q?: string; page: number }) {
  return measured("master.listUniversities", async (): Promise<AdminUniversityList> => {
    const where = params.q ? "WHERE u.name LIKE ?" : "";
    const whereParams = params.q ? [likePattern(params.q)] : [];
    const [{ total }] = await select<{ total: number | string }>(
      `SELECT COUNT(*) AS total FROM University u ${where}`,
      whereParams
    );
    const rows = await select<UniversityRow & CountRow>(
      `SELECT ${UNIVERSITY_COLUMNS} FROM University u ${where}
       ORDER BY u.name ASC, u.id ASC LIMIT ? OFFSET ?`,
      [...whereParams, ADMIN_UNIVERSITIES_PAGE_SIZE, (params.page - 1) * ADMIN_UNIVERSITIES_PAGE_SIZE]
    );
    return {
      universities: rows.map(toAdminUniversity),
      total: Number(total),
      page: params.page,
      pageSize: ADMIN_UNIVERSITIES_PAGE_SIZE,
    };
  });
}

async function findAdminUniversity(id: number, db?: PoolConnection) {
  const [row] = await select<UniversityRow & CountRow>(
    `SELECT ${UNIVERSITY_COLUMNS} FROM University u WHERE u.id = ?`,
    [id],
    db
  );
  return row ? toAdminUniversity(row) : null;
}

type FacultyWithTagRow = Pick<FacultyRow, "id" | "name" | "examDate"> & {
  goalCount: number | string;
  tagId: number | null;
  tagName: string | null;
};

/** 大学と、その学部（タグ・志望校にしている件数つき）。 */
export function getAdminUniversityDetail(id: number) {
  return measured("master.universityDetail", async (): Promise<AdminUniversityDetail | null> => {
    const university = await findAdminUniversity(id);
    if (!university) return null;

    const rows = await select<FacultyWithTagRow>(
      `SELECT f.id, f.name, f.examDate,
              (SELECT COUNT(*) FROM FinalGoal g WHERE g.facultyId = f.id) AS goalCount,
              t.id AS tagId, t.name AS tagName
       FROM Faculty f
       LEFT JOIN _FacultyToTag ft ON ft.A = f.id
       LEFT JOIN Tag t ON t.id = ft.B
       WHERE f.universityId = ?
       ORDER BY f.id ASC, t.id ASC`,
      [id]
    );

    // 行は（学部 × タグ）の数だけ並ぶ。同じ学部の行は隣り合うので、直前と比べて束ねる。
    const faculties: AdminFaculty[] = [];
    for (const row of rows) {
      let faculty = faculties.at(-1);
      if (faculty?.id !== row.id) {
        faculty = {
          id: row.id,
          name: row.name,
          examDate: row.examDate.toISOString().slice(0, 10),
          tags: [],
          goalCount: Number(row.goalCount),
        };
        faculties.push(faculty);
      }
      if (row.tagId !== null && row.tagName !== null) faculty.tags.push({ id: row.tagId, name: row.tagName });
    }
    return { university, faculties };
  });
}

export function listAdminTags() {
  return measured("master.listTags", async (): Promise<AdminTag[]> =>
    select<Pick<TagRow, "id" | "name">>("SELECT id, name FROM Tag ORDER BY id ASC")
  );
}

export function createUniversity(input: UniversityInput) {
  return measured("master.createUniversity", async (): Promise<Outcome<AdminUniversity>> => {
    try {
      const created = await execute(
        "INSERT INTO University (name, prefecture, type, createdAt) VALUES (?, ?, ?, ?)",
        [input.name, input.prefecture, input.type, new Date()]
      );
      return { result: "ok", value: (await findAdminUniversity(created.insertId))! };
    } catch (error) {
      if (isDuplicateEntry(error)) return { result: "duplicate" };
      throw error;
    }
  });
}

export function updateUniversity(id: number, input: UniversityInput) {
  return measured(
    "master.updateUniversity",
    async (): Promise<Outcome<{ before: AdminUniversity; after: AdminUniversity }>> => {
      const before = await findAdminUniversity(id);
      if (!before) return { result: "not_found" };
      try {
        await execute("UPDATE University SET name = ?, prefecture = ?, type = ? WHERE id = ?", [
          input.name,
          input.prefecture,
          input.type,
          id,
        ]);
      } catch (error) {
        if (isDuplicateEntry(error)) return { result: "duplicate" };
        throw error;
      }
      return { result: "ok", value: { before, after: (await findAdminUniversity(id))! } };
    }
  );
}

export function deleteUniversity(id: number) {
  return measured("master.deleteUniversity", async (): Promise<Outcome<AdminUniversity>> => {
    const university = await findAdminUniversity(id);
    if (!university) return { result: "not_found" };
    if (university.goalCount > 0) return { result: "in_use", goalCount: university.goalCount };
    try {
      await execute("DELETE FROM University WHERE id = ?", [id]);
    } catch (error) {
      if (isReferenced(error)) return { result: "in_use", goalCount: university.goalCount };
      throw error;
    }
    return { result: "ok", value: university };
  });
}

type FacultySnapshot = { id: number; universityId: number; name: string; examDate: string; tagIds: number[] };

async function findFacultySnapshot(id: number, db?: PoolConnection): Promise<FacultySnapshot | null> {
  const [row] = await select<Pick<FacultyRow, "id" | "universityId" | "name" | "examDate">>(
    "SELECT id, universityId, name, examDate FROM Faculty WHERE id = ?",
    [id],
    db
  );
  if (!row) return null;
  const tags = await select<{ tagId: number }>(
    "SELECT B AS tagId FROM _FacultyToTag WHERE A = ? ORDER BY B ASC",
    [id],
    db
  );
  return {
    id: row.id,
    universityId: row.universityId,
    name: row.name,
    examDate: row.examDate.toISOString().slice(0, 10),
    tagIds: tags.map((tag) => tag.tagId),
  };
}

// Faculty には (universityId, name) の一意制約が無い（seed が名前で照合している）ので、ここで重複を断る。
async function hasFacultyNamed(db: PoolConnection, universityId: number, name: string, exceptId?: number) {
  const rows = await select<{ id: number }>(
    "SELECT id FROM Faculty WHERE universityId = ? AND name = ? AND id <> ? LIMIT 1",
    [universityId, name, exceptId ?? 0],
    db
  );
  return rows.length > 0;
}

async function existingTagCount(db: PoolConnection, tagIds: number[]) {
  if (tagIds.length === 0) return 0;
  const [{ count }] = await select<{ count: number | string }>(
    "SELECT COUNT(*) AS count FROM Tag WHERE id IN (?)",
    [tagIds],
    db
  );
  return Number(count);
}

async function replaceTags(db: PoolConnection, facultyId: number, tagIds: number[]) {
  // 中間テーブルの A = Faculty.id, B = Tag.id（seed.ts と同じく置き換え）
  await execute("DELETE FROM _FacultyToTag WHERE A = ?", [facultyId], db);
  if (tagIds.length > 0) {
    await execute(
      `INSERT INTO _FacultyToTag (A, B) VALUES ${tagIds.map(() => "(?, ?)").join(", ")}`,
      tagIds.flatMap((tagId) => [facultyId, tagId]),
      db
    );
  }
}

export function createFaculty(input: CreateFacultyInput) {
  return measured("master.createFaculty", async (): Promise<Outcome<FacultySnapshot>> =>
    transaction(async (tx) => {
      const [university] = await select<{ id: number }>(
        "SELECT id FROM University WHERE id = ? FOR UPDATE",
        [input.universityId],
        tx
      );
      if (!university) return { result: "not_found" };
      if (await hasFacultyNamed(tx, input.universityId, input.name)) return { result: "duplicate" };
      if ((await existingTagCount(tx, input.tagIds)) !== input.tagIds.length) return { result: "invalid_tags" };

      const created = await execute(
        "INSERT INTO Faculty (name, examDate, universityId, createdAt) VALUES (?, ?, ?, ?)",
        [input.name, new Date(input.examDate), input.universityId, new Date()],
        tx
      );
      await replaceTags(tx, created.insertId, input.tagIds);
      return { result: "ok", value: (await findFacultySnapshot(created.insertId, tx))! };
    })
  );
}

export function updateFaculty(id: number, input: FacultyInput) {
  return measured(
    "master.updateFaculty",
    async (): Promise<Outcome<{ before: FacultySnapshot; after: FacultySnapshot }>> =>
      transaction(async (tx) => {
        const before = await findFacultySnapshot(id, tx);
        if (!before) return { result: "not_found" };
        if (await hasFacultyNamed(tx, before.universityId, input.name, id)) return { result: "duplicate" };
        if ((await existingTagCount(tx, input.tagIds)) !== input.tagIds.length) return { result: "invalid_tags" };

        await execute("UPDATE Faculty SET name = ?, examDate = ? WHERE id = ?", [
          input.name,
          new Date(input.examDate),
          id,
        ], tx);
        await replaceTags(tx, id, input.tagIds);
        return { result: "ok", value: { before, after: (await findFacultySnapshot(id, tx))! } };
      })
  );
}

export function deleteFaculty(id: number) {
  return measured("master.deleteFaculty", async (): Promise<Outcome<FacultySnapshot>> => {
    const faculty = await findFacultySnapshot(id);
    if (!faculty) return { result: "not_found" };
    const [{ goalCount }] = await select<{ goalCount: number | string }>(
      "SELECT COUNT(*) AS goalCount FROM FinalGoal WHERE facultyId = ?",
      [id]
    );
    if (Number(goalCount) > 0) return { result: "in_use", goalCount: Number(goalCount) };
    try {
      // 中間テーブルの行は外部キーの CASCADE で一緒に消える。
      await execute("DELETE FROM Faculty WHERE id = ?", [id]);
    } catch (error) {
      if (isReferenced(error)) return { result: "in_use", goalCount: Number(goalCount) };
      throw error;
    }
    return { result: "ok", value: faculty };
  });
}
