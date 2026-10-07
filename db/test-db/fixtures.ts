import { randomUUID } from "node:crypto";
import { execute, pool } from "../connection";

// 本物の DB に流すテストの下ごしらえ。
//
// テストごとに使い捨てのユーザーを作り、データはすべてそのユーザーにぶら下げる。
// テーブルを空にしてから始める方式だと、並列に走る別のテストファイルの行まで消してしまう。
// ユーザーで分けておけば、互いの行が見えないので並列でも干渉しない。
//
// 志望校・予定・参考書・マスターを作る関数は、それを使う Node の API とテストを消したときに一緒に消した（JUK-84）。
// Go の DB テストの下ごしらえは apps/api/internal/dbtest/dbtest.go にある。

const createdUserIds: string[] = [];

export async function createUser() {
  const id = `test-${randomUUID()}`;
  const now = new Date();
  await execute(
    "INSERT INTO `user` (id, email, createdAt, updatedAt) VALUES (?, ?, ?, ?)",
    [id, `${id}@example.test`, now, now]
  );
  createdUserIds.push(id);
  return { id };
}

/**
 * afterAll で呼ぶ。作ったユーザーを消し、接続を閉じる。
 * ユーザーにぶら下がる行は、外部キーの CASCADE で一緒に消える。
 */
export async function cleanup() {
  if (createdUserIds.length > 0) {
    await execute("DELETE FROM `user` WHERE id IN (?)", [createdUserIds.splice(0)]);
  }
  await pool.end();
}
