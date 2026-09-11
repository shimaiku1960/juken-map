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
  values: { studyPlanId?: number | null; minutes?: number; date?: Date } = {}
) {
  const now = new Date();
  const result = await execute(
    `INSERT INTO StudyLog (userId, studyPlanId, date, minutes, createdAt, updatedAt)
     VALUES (?, ?, ?, ?, ?, ?)`,
    [userId, values.studyPlanId ?? null, values.date ?? now, values.minutes ?? 30, now, now]
  );
  return result.insertId;
}

// 検証用の読み取り。応答だけでなく、DB に実際に何が残ったかを確かめるのに使う。

export async function findStudyPlan(id: number) {
  const [row] = await select<StudyPlanRow>("SELECT * FROM StudyPlan WHERE id = ?", [id]);
  return row ?? null;
}

export function findStudyLogs(userId: string) {
  return select<StudyLogRow>("SELECT * FROM StudyLog WHERE userId = ? ORDER BY id", [userId]);
}

export async function findUser(id: string) {
  const [row] = await select<{ id: string; firstStudyLogAt: Date | null }>(
    "SELECT id, firstStudyLogAt FROM `user` WHERE id = ?",
    [id]
  );
  return row ?? null;
}

/** afterAll で呼ぶ。作ったユーザーを消し（予定・実績・参考書は外部キーの CASCADE で消える）、接続を閉じる。 */
export async function cleanup() {
  if (createdUserIds.length > 0) {
    await execute("DELETE FROM `user` WHERE id IN (?)", [createdUserIds.splice(0)]);
  }
  await pool.end();
}
