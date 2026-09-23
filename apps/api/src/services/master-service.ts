import type { PoolConnection } from "mysql2/promise";
import { execute, isDuplicateEntry, select, transaction } from "@/api/infra/db";
import type { FacultyRow, TagRow, TextbookMasterRow, UniversityRow } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";
import { invalidateUniversitiesForExplore } from "@/api/services/university-service";
import type {
  AdminFaculty,
  AdminTag,
  AdminTextbookMaster,
  AdminUniversity,
  AdminUniversityDetail,
  AdminUniversityList,
} from "@/shared/dto/admin";
import type {
  CreateFacultyInput,
  FacultyInput,
  TextbookMasterInput,
  UniversityInput,
} from "@/shared/validations/master";

// 管理者ページのマスター編集（大学・学部・参考書）。
//
// 削除は「誰にも使われていない行」に限る。
//   - 学部：志望校（FinalGoal）が参照していれば消せない（DB も ON DELETE RESTRICT で拒む）
//   - 大学：学部は CASCADE で一緒に消えるので、配下の学部がどれか志望校に使われていれば止める
//   - 参考書：利用者の参考書（Textbook.masterId）は SET NULL で黙って紐づきが外れるので、1冊でもあれば止める
// 事前に数えて断るのが基本で、数えたあとに志望校が増えた場合も、DB の外部キーが拒んだエラーを
// 同じ「使われている」に読み替える（二重の守り）。
//
// 大学・学部を変えたら、利用者向けの大学一覧のキャッシュを捨てる（university-service 参照）。
// 変わったときだけ捨てればよいので、成功（ok）のときに限る。

export const ADMIN_UNIVERSITIES_PAGE_SIZE = 50;

type Outcome<T> =
  | { result: "ok"; value: T }
  | { result: "not_found" }
  | { result: "duplicate" }
  | { result: "in_use"; count: number }
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
      invalidateUniversitiesForExplore();
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
      invalidateUniversitiesForExplore();
      return { result: "ok", value: { before, after: (await findAdminUniversity(id))! } };
    }
  );
}

export function deleteUniversity(id: number) {
  return measured("master.deleteUniversity", async (): Promise<Outcome<AdminUniversity>> => {
    const university = await findAdminUniversity(id);
    if (!university) return { result: "not_found" };
    if (university.goalCount > 0) return { result: "in_use", count: university.goalCount };
    try {
      await execute("DELETE FROM University WHERE id = ?", [id]);
    } catch (error) {
      if (isReferenced(error)) return { result: "in_use", count: university.goalCount };
      throw error;
    }
    invalidateUniversitiesForExplore();
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

// トランザクションの中で捨てると、確定前に別のリクエストが古い一覧を読み直して置き直せる。
// 確定（commit）してから捨てる。
async function afterCommit<T extends { result: string }>(committed: Promise<T>) {
  const outcome = await committed;
  if (outcome.result === "ok") invalidateUniversitiesForExplore();
  return outcome;
}

export function createFaculty(input: CreateFacultyInput) {
  return measured("master.createFaculty", async (): Promise<Outcome<FacultySnapshot>> =>
    afterCommit(transaction(async (tx): Promise<Outcome<FacultySnapshot>> => {
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
    }))
  );
}

export function updateFaculty(id: number, input: FacultyInput) {
  return measured(
    "master.updateFaculty",
    async (): Promise<Outcome<{ before: FacultySnapshot; after: FacultySnapshot }>> =>
      afterCommit(transaction(async (tx): Promise<Outcome<{ before: FacultySnapshot; after: FacultySnapshot }>> => {
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
      }))
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
    if (Number(goalCount) > 0) return { result: "in_use", count: Number(goalCount) };
    try {
      // 中間テーブルの行は外部キーの CASCADE で一緒に消える。
      await execute("DELETE FROM Faculty WHERE id = ?", [id]);
    } catch (error) {
      if (isReferenced(error)) return { result: "in_use", count: Number(goalCount) };
      throw error;
    }
    invalidateUniversitiesForExplore();
    return { result: "ok", value: faculty };
  });
}

// ---- 参考書 ----

type TextbookMasterListRow = Pick<TextbookMasterRow, "id" | "name" | "publisher" | "edition" | "isbn"> & {
  textbookCount: number | string;
  unit: string | null;
  totalAmount: number | null;
  isDefault: boolean | null;
};

const ADMIN_TEXTBOOK_MASTERS_LIMIT = 200;

async function selectAdminTextbookMasters(where: string, params: unknown[], db?: PoolConnection) {
  const rows = await select<TextbookMasterListRow>(
    `SELECT tm.id, tm.name, tm.publisher, tm.edition, tm.isbn,
            (SELECT COUNT(*) FROM Textbook t WHERE t.masterId = tm.id) AS textbookCount,
            m.unit, m.totalAmount, m.isDefault
     FROM (SELECT * FROM TextbookMaster tm ${where} ORDER BY tm.id ASC LIMIT ${ADMIN_TEXTBOOK_MASTERS_LIMIT}) AS tm
     LEFT JOIN TextbookMasterMetric m ON m.masterId = tm.id
     ORDER BY tm.id ASC, m.id ASC`,
    params,
    db
  );
  // 行は（参考書 × 総量の候補）の数だけ並ぶ。同じ参考書の行は隣り合うので、直前と比べて束ねる。
  const masters: AdminTextbookMaster[] = [];
  for (const row of rows) {
    let master = masters.at(-1);
    if (master?.id !== row.id) {
      master = {
        id: row.id,
        name: row.name,
        publisher: row.publisher,
        edition: row.edition,
        isbn: row.isbn,
        metrics: [],
        textbookCount: Number(row.textbookCount),
      };
      masters.push(master);
    }
    if (row.unit !== null) {
      master.metrics.push({ unit: row.unit, totalAmount: row.totalAmount!, isDefault: row.isDefault! });
    }
  }
  return masters;
}

async function findAdminTextbookMaster(id: number, db?: PoolConnection) {
  const [master] = await selectAdminTextbookMasters("WHERE tm.id = ?", [id], db);
  return master ?? null;
}

/** 参考書マスターの一覧（名前・出版社・ISBN の部分一致）。件数は少ないので先頭200件まで。 */
export function listAdminTextbookMasters(q?: string) {
  return measured("master.listTextbookMasters", async () =>
    q
      ? selectAdminTextbookMasters("WHERE tm.name LIKE ? OR tm.publisher LIKE ? OR tm.isbn LIKE ?", [
          likePattern(q),
          likePattern(q),
          likePattern(q),
        ])
      : selectAdminTextbookMasters("", [])
  );
}

async function replaceMetrics(db: PoolConnection, masterId: number, metrics: TextbookMasterInput["metrics"]) {
  await execute("DELETE FROM TextbookMasterMetric WHERE masterId = ?", [masterId], db);
  const now = new Date();
  await execute(
    `INSERT INTO TextbookMasterMetric (masterId, unit, totalAmount, isDefault, createdAt, updatedAt)
     VALUES ${metrics.map(() => "(?, ?, ?, ?, ?, ?)").join(", ")}`,
    metrics.flatMap((metric) => [masterId, metric.unit, metric.totalAmount, metric.isDefault, now, now]),
    db
  );
}

export function createTextbookMaster(input: TextbookMasterInput) {
  return measured("master.createTextbookMaster", async (): Promise<Outcome<AdminTextbookMaster>> => {
    try {
      return await transaction(async (tx) => {
        const now = new Date();
        const created = await execute(
          `INSERT INTO TextbookMaster (name, publisher, edition, isbn, createdAt, updatedAt)
           VALUES (?, ?, ?, ?, ?, ?)`,
          [input.name, input.publisher, input.edition, input.isbn, now, now],
          tx
        );
        await replaceMetrics(tx, created.insertId, input.metrics);
        return { result: "ok" as const, value: (await findAdminTextbookMaster(created.insertId, tx))! };
      });
    } catch (error) {
      if (isDuplicateEntry(error)) return { result: "duplicate" };
      throw error;
    }
  });
}

/**
 * 参考書マスターを書き換える。利用者がすでに登録した参考書（Textbook）は総量を自分の行に
 * 写し取っているので、ここで総量を変えても既存の利用者の参考書は変わらない（これから登録する人から効く）。
 */
export function updateTextbookMaster(id: number, input: TextbookMasterInput) {
  return measured(
    "master.updateTextbookMaster",
    async (): Promise<Outcome<{ before: AdminTextbookMaster; after: AdminTextbookMaster }>> => {
      try {
        return await transaction(async (tx) => {
          const before = await findAdminTextbookMaster(id, tx);
          if (!before) return { result: "not_found" as const };
          await execute(
            "UPDATE TextbookMaster SET name = ?, publisher = ?, edition = ?, isbn = ?, updatedAt = ? WHERE id = ?",
            [input.name, input.publisher, input.edition, input.isbn, new Date(), id],
            tx
          );
          await replaceMetrics(tx, id, input.metrics);
          return { result: "ok" as const, value: { before, after: (await findAdminTextbookMaster(id, tx))! } };
        });
      } catch (error) {
        if (isDuplicateEntry(error)) return { result: "duplicate" };
        throw error;
      }
    }
  );
}

export function deleteTextbookMaster(id: number) {
  return measured("master.deleteTextbookMaster", async (): Promise<Outcome<AdminTextbookMaster>> => {
    const master = await findAdminTextbookMaster(id);
    if (!master) return { result: "not_found" };
    if (master.textbookCount > 0) return { result: "in_use", count: master.textbookCount };
    // 総量の候補は外部キーの CASCADE で一緒に消える。
    await execute("DELETE FROM TextbookMaster WHERE id = ?", [id]);
    return { result: "ok", value: master };
  });
}
