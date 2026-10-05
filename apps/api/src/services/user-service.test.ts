import { randomBytes, randomUUID } from "node:crypto";
import { afterAll, describe, expect, it } from "vitest";
import { execute, pool, select } from "@/api/infra/db";
import { setUserRoleByEmail } from "./user-service.ts";

// pnpm admin:grant（管理者の付け外し）を本物の DB で確かめる。

const userIds: string[] = [];

afterAll(async () => {
  if (userIds.length > 0) await execute("DELETE FROM `user` WHERE id IN (?)", [userIds]);
  await pool.end();
});

async function verifiedUserWithSessions(sessions: number, emailVerified = true) {
  const id = `grant-${randomUUID()}`;
  const email = `${id}@example.test`;
  const now = new Date();
  await execute(
    "INSERT INTO `user` (id, email, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)",
    [id, email, emailVerified, now, now]
  );
  userIds.push(id);
  for (let i = 0; i < sessions; i++) {
    await execute(
      "INSERT INTO AuthSession (id, tokenHash, userId, createdAt, expiresAt, lastUsedAt) VALUES (?, ?, ?, ?, ?, ?)",
      [randomBytes(16).toString("hex"), randomBytes(32), id, now, new Date(now.getTime() + 86_400_000), now]
    );
  }
  return { id, email };
}

async function sessionCount(userId: string) {
  const [row] = await select<{ n: number }>("SELECT COUNT(*) AS n FROM AuthSession WHERE userId = ?", [userId]);
  return Number(row.n);
}

describe("setUserRoleByEmail", () => {
  it("付け替えたら、その人のセッションをすべて消す（認証基準 10 の C4：権限の変更で作り直す）", async () => {
    const user = await verifiedUserWithSessions(2);
    const other = await verifiedUserWithSessions(1);

    const outcome = await setUserRoleByEmail(user.email, "admin");

    expect(outcome).toEqual({ result: "updated", previous: "user", sessionsRemoved: 2 });
    expect(await sessionCount(user.id)).toBe(0);
    expect(await sessionCount(other.id)).toBe(1);
  });

  it("メール確認前の人は管理者にせず、セッションも消さない", async () => {
    const user = await verifiedUserWithSessions(1, false);

    const outcome = await setUserRoleByEmail(user.email, "admin");

    expect(outcome).toEqual({ result: "unverified" });
    expect(await sessionCount(user.id)).toBe(1);
  });
});
