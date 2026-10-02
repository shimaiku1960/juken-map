import { randomUUID } from "node:crypto";
import { afterAll, describe, expect, it } from "vitest";
import { execute, pool, select } from "@/api/infra/db";
import {
  banByEmail,
  listSessionsByEmail,
  resetTwoFactorByEmail,
  revokeAdminSessions,
  revokeSessionsByEmail,
  unbanByEmail,
} from "./incident-service.ts";

// pnpm incident（乗っ取りが起きたときの操作）を本物の DB で確かめる。
// 本番では手順書（docs/incident-response.md）からしか使わないので、いざというときに
// SQL の誤りで動かない、ということがないようにする。

const userIds: string[] = [];

afterAll(async () => {
  if (userIds.length > 0) await execute("DELETE FROM `user` WHERE id IN (?)", [userIds]);
  await pool.end();
});

/** 利用者を作り、セッションを sessions 件ぶら下げる。 */
async function userWithSessions(sessions: number, role: "user" | "admin" = "user") {
  const id = `incident-${randomUUID()}`;
  const email = `${id}@example.test`;
  const now = new Date();
  await execute(
    "INSERT INTO `user` (id, email, role, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)",
    [id, email, role, now, now]
  );
  userIds.push(id);
  for (let i = 0; i < sessions; i++) {
    await execute(
      "INSERT INTO session (id, userId, token, expiresAt, updatedAt, ipAddress) VALUES (?, ?, ?, ?, ?, ?)",
      [randomUUID(), id, randomUUID(), new Date(now.getTime() + 86_400_000), now, `192.0.2.${i}`]
    );
  }
  return { id, email };
}

async function sessionCount(userId: string) {
  const [row] = await select<{ n: number }>(
    "SELECT COUNT(*) AS n FROM session WHERE userId = ?",
    [userId]
  );
  return Number(row.n);
}

async function bannedAt(userId: string) {
  const [row] = await select<{ bannedAt: Date | null }>(
    "SELECT bannedAt FROM `user` WHERE id = ?",
    [userId]
  );
  return row.bannedAt;
}

describe("pnpm incident", () => {
  it("sessions は、その人のセッションを作られた順に返す", async () => {
    const target = await userWithSessions(2);

    const found = await listSessionsByEmail(target.email);

    expect(found?.sessions.map((s) => s.ipAddress).sort()).toEqual(["192.0.2.0", "192.0.2.1"]);
  });

  it("revoke は、その人のセッションだけを消し、止めはしない", async () => {
    const target = await userWithSessions(2);
    const other = await userWithSessions(1);

    const done = await revokeSessionsByEmail(target.email);

    expect(done?.sessionsRemoved).toBe(2);
    expect(await sessionCount(target.id)).toBe(0);
    expect(await sessionCount(other.id)).toBe(1);
    expect(await bannedAt(target.id)).toBeNull();
  });

  it("ban は、管理者でも止めてセッションを消し、止め直しても最初の日時を保つ", async () => {
    const admin = await userWithSessions(2, "admin");

    const done = await banByEmail(admin.email);
    const first = await bannedAt(admin.id);
    await banByEmail(admin.email);

    expect(done?.sessionsRemoved).toBe(2);
    expect(await sessionCount(admin.id)).toBe(0);
    expect(first).not.toBeNull();
    expect(await bannedAt(admin.id)).toEqual(first);
  });

  it("unban は、止めたのを戻す", async () => {
    const target = await userWithSessions(0);
    await banByEmail(target.email);

    await unbanByEmail(target.email);

    expect(await bannedAt(target.id)).toBeNull();
  });

  it("revoke-admins は、管理者のセッションだけを全員分消す", async () => {
    const adminA = await userWithSessions(1, "admin");
    const adminB = await userWithSessions(2, "admin");
    const user = await userWithSessions(1);

    const done = await revokeAdminSessions();

    expect(done.admins.map((a) => a.id)).toEqual(expect.arrayContaining([adminA.id, adminB.id]));
    expect(await sessionCount(adminA.id)).toBe(0);
    expect(await sessionCount(adminB.id)).toBe(0);
    expect(await sessionCount(user.id)).toBe(1);
  });

  it("reset-2fa は、2段階認証を設定前に戻し、セッションを消す", async () => {
    const admin = await userWithSessions(1, "admin");
    await execute("UPDATE `user` SET twoFactorEnabled = true WHERE id = ?", [admin.id]);
    await execute(
      "INSERT INTO twoFactor (id, secret, backupCodes, userId) VALUES (?, 'secret', 'codes', ?)",
      [randomUUID(), admin.id]
    );

    const done = await resetTwoFactorByEmail(admin.email);

    const [user] = await select<{ twoFactorEnabled: number }>(
      "SELECT twoFactorEnabled FROM `user` WHERE id = ?",
      [admin.id]
    );
    const rows = await select<{ id: string }>("SELECT id FROM twoFactor WHERE userId = ?", [
      admin.id,
    ]);
    expect(done?.sessionsRemoved).toBe(1);
    expect(Boolean(user.twoFactorEnabled)).toBe(false);
    expect(rows).toHaveLength(0);
    expect(await sessionCount(admin.id)).toBe(0);
  });

  it("いないメールアドレスなら何もせず undefined を返す", async () => {
    const email = `missing-${randomUUID()}@example.test`;

    expect(await listSessionsByEmail(email)).toBeUndefined();
    expect(await revokeSessionsByEmail(email)).toBeUndefined();
    expect(await banByEmail(email)).toBeUndefined();
    expect(await unbanByEmail(email)).toBeUndefined();
    expect(await resetTwoFactorByEmail(email)).toBeUndefined();
  });
});
