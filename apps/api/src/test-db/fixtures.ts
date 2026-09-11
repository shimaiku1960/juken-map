import { randomUUID } from "node:crypto";
import { execute, pool, select } from "@/api/infra/db";
import type { StudyLogRow, StudyPlanRow } from "@/api/infra/tables";

// 本物の DB に流すテストの下ごしらえ。
//
// テストごとに使い捨てのユーザーを作り、データはすべてそのユーザーにぶら下げる。
// テーブルを空にしてから始める方式だと、並列に走る別のテストファイルの行まで消してしまう。
// ユーザーで分けておけば、互いの行が見えないので並列でも干渉しない。

const createdUserIds: string[] = [];

export async function createUser() {
  const id = `test-${randomUUID()}`;
  const now = new Date();
  await execute(
    "INSERT INTO `user` (id, email, createdAt, updatedAt) VALUES (?, ?, ?, ?)",
    [id, `${id}@example.test`, now, now]
  );
  createdUserIds.push(id);
  // ルートが見るのは session.user.id と email だけ（test-support.ts と同じ形）。
  return { id, session: { user: { id, email: `${id}@example.test` } } };
}

export async function createTextbook(
  userId: string,
  values: { name?: string; totalAmount?: number | null; rangeUnit?: string | null } = {}
) {
  const now = new Date();
  const result = await execute(
    `INSERT INTO Textbook (userId, name, totalAmount, rangeUnit, createdAt, updatedAt)
     VALUES (?, ?, ?, ?, ?, ?)`,
    [userId, values.name ?? "参考書", values.totalAmount ?? null, values.rangeUnit ?? null, now, now]
  );
  return result.insertId;
}

export async function createStudyPlan(
  userId: string,
  values: Partial<Omit<StudyPlanRow, "id" | "userId" | "createdAt" | "updatedAt">> = {}
) {
  const now = new Date();
  const result = await execute(
    `INSERT INTO StudyPlan
       (userId, date, content, subject, done, textbookId,
        rangeStart, rangeEnd, rangeUnit, createdAt, updatedAt)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    [
      userId,
      values.date ?? new Date("2027-02-20T00:00:00.000Z"),
      values.content ?? null,
      values.subject ?? null,
      values.done ?? false,
      values.textbookId ?? null,
      values.rangeStart ?? null,
      values.rangeEnd ?? null,
      values.rangeUnit ?? null,
      now,
      now,
    ]
  );
  return result.insertId;
}

export async function createStudyLog(
  userId: string,
  values: Partial<Omit<StudyLogRow, "id" | "userId" | "createdAt" | "updatedAt">> = {}
) {
  const now = new Date();
  const result = await execute(
    `INSERT INTO StudyLog
       (userId, studyPlanId, date, minutes, subject, textbookId,
        rangeStart, rangeEnd, rangeUnit, memo, createdAt, updatedAt)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    [
      userId,
      values.studyPlanId ?? null,
      values.date ?? now,
      values.minutes ?? 30,
      values.subject ?? null,
      values.textbookId ?? null,
      values.rangeStart ?? null,
      values.rangeEnd ?? null,
      values.rangeUnit ?? null,
      values.memo ?? null,
      now,
      now,
    ]
  );
  return result.insertId;
}

/** Better Auth が作る認証アカウント。providerId はメール登録なら "credential"。 */
export async function createAccount(
  userId: string,
  providerId: string,
  createdAt = new Date()
) {
  await execute(
    `INSERT INTO account (id, userId, accountId, providerId, createdAt, updatedAt)
     VALUES (?, ?, ?, ?, ?, ?)`,
    [randomUUID(), userId, randomUUID(), providerId, createdAt, createdAt]
  );
}

export async function createNotificationPreference(
  userId: string,
  values: {
    morningEnabled?: boolean;
    eveningEnabled?: boolean;
    lineMorningEnabled?: boolean;
    lineEveningEnabled?: boolean;
  }
) {
  const now = new Date();
  await execute(
    `INSERT INTO NotificationPreference
       (userId, morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled, createdAt, updatedAt)
     VALUES (?, ?, ?, ?, ?, ?, ?)`,
    [
      userId,
      values.morningEnabled ?? false,
      values.eveningEnabled ?? false,
      values.lineMorningEnabled ?? false,
      values.lineEveningEnabled ?? false,
      now,
      now,
    ]
  );
}

export async function createLineConnection(userId: string, lineUserId = `U${randomUUID()}`) {
  const now = new Date();
  await execute(
    "INSERT INTO LineConnection (userId, lineUserId, linkedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)",
    [userId, lineUserId, now, now, now]
  );
  return lineUserId;
}

// 大学・学部・タグはマスターデータで、ユーザーにぶら下がらない。
// 名前に UNIQUE 制約があるので、並列のテストとぶつからないよう乱数を付ける。
const createdUniversityIds: number[] = [];
const createdTagIds: number[] = [];

export async function createTag(name = `タグ-${randomUUID()}`) {
  const result = await execute("INSERT INTO Tag (name, createdAt) VALUES (?, ?)", [name, new Date()]);
  createdTagIds.push(result.insertId);
  return result.insertId;
}

export async function createUniversity(values: {
  name?: string;
  faculties?: { name?: string; examDate?: Date; tagIds?: number[] }[];
} = {}) {
  const university = await execute(
    "INSERT INTO University (name, prefecture, type, createdAt) VALUES (?, ?, ?, ?)",
    [values.name ?? `大学-${randomUUID()}`, "東京都", "私立", new Date()]
  );
  createdUniversityIds.push(university.insertId);

  const facultyIds: number[] = [];
  for (const faculty of values.faculties ?? []) {
    const created = await execute(
      "INSERT INTO Faculty (name, examDate, universityId, createdAt) VALUES (?, ?, ?, ?)",
      [faculty.name ?? "学部", faculty.examDate ?? new Date("2027-02-15T00:00:00.000Z"), university.insertId, new Date()]
    );
    facultyIds.push(created.insertId);
    for (const tagId of faculty.tagIds ?? []) {
      // Prisma の暗黙の多対多の中間テーブル。A が Faculty、B が Tag。
      await execute("INSERT INTO _FacultyToTag (A, B) VALUES (?, ?)", [created.insertId, tagId]);
    }
  }
  return { id: university.insertId, facultyIds };
}

export async function createFinalGoal(userId: string, facultyId: number) {
  await execute(
    "INSERT INTO FinalGoal (userId, facultyId, createdAt) VALUES (?, ?, ?)",
    [userId, facultyId, new Date()]
  );
}

// 検証用の読み取り。応答だけでなく、DB に実際に何が残ったかを確かめるのに使う。

export async function findStudyPlan(id: number) {
  const [row] = await select<StudyPlanRow>("SELECT * FROM StudyPlan WHERE id = ?", [id]);
  return row ?? null;
}

export async function findStudyLog(id: number) {
  const [row] = await select<StudyLogRow>("SELECT * FROM StudyLog WHERE id = ?", [id]);
  return row ?? null;
}

export function findStudyLogs(userId: string) {
  return select<StudyLogRow>("SELECT * FROM StudyLog WHERE userId = ? ORDER BY id", [userId]);
}

export async function findUser(id: string) {
  const [row] = await select<{
    id: string;
    nickname: string | null;
    firstStudyLogAt: Date | null;
    analyticsSignUpTrackedAt: Date | null;
  }>(
    "SELECT id, nickname, firstStudyLogAt, analyticsSignUpTrackedAt FROM `user` WHERE id = ?",
    [id]
  );
  return row ?? null;
}

export function findNotificationPreferences(userId: string) {
  return select<{
    morningEnabled: boolean;
    eveningEnabled: boolean;
    lineMorningEnabled: boolean;
    lineEveningEnabled: boolean;
    createdAt: Date;
    updatedAt: Date;
  }>(
    `SELECT morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled, createdAt, updatedAt
     FROM NotificationPreference WHERE userId = ?`,
    [userId]
  );
}

export function findNotificationDeliveries(userId: string) {
  return select<{ id: number; date: Date; slot: string; channel: string }>(
    "SELECT id, date, slot, channel FROM NotificationDelivery WHERE userId = ? ORDER BY id",
    [userId]
  );
}

/**
 * afterAll で呼ぶ。作ったものを消し、接続を閉じる。
 *
 * ユーザーを先に消す。志望校（FinalGoal）は学部を ON DELETE RESTRICT で参照していて、
 * 志望校が残っていると大学（→学部）を消せない。ユーザーを消せば志望校・予定・実績などは
 * 外部キーの CASCADE で一緒に消える。大学を消せば学部と中間テーブルも消える。
 */
export async function cleanup() {
  if (createdUserIds.length > 0) {
    await execute("DELETE FROM `user` WHERE id IN (?)", [createdUserIds.splice(0)]);
  }
  if (createdUniversityIds.length > 0) {
    await execute("DELETE FROM University WHERE id IN (?)", [createdUniversityIds.splice(0)]);
  }
  if (createdTagIds.length > 0) {
    await execute("DELETE FROM Tag WHERE id IN (?)", [createdTagIds.splice(0)]);
  }
  await pool.end();
}
