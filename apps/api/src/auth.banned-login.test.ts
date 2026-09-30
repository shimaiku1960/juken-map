import { randomUUID } from "node:crypto";
import { afterAll, describe, expect, it, vi } from "vitest";

// 確認メール・登録通知は送らない。Better Auth 本体は本物を通す。
vi.mock("@/api/infra/email", () => ({
  notifyAdminOfNewUser: vi.fn(),
  sendVerificationEmail: vi.fn(),
  sendPasswordResetEmail: vi.fn(),
}));

const { auth } = await import("./auth.ts");
const { execute, pool } = await import("@/api/infra/db");

// 停止の分担（JUK-78）。停止の API は Go（apps/api-go の admin_users.go）が受け持ち、
// user.bannedAt を書いてその人の session を消すだけ。次のログインを断るのは、ログインを発行する
// Node（auth.ts の databaseHooks.session.create.before）の役目。
// ここでは、Go が書くのと同じ形で bannedAt を入れた利用者が、Better Auth でログインできないことを確かめる。
// Go 側（bannedAt を書く・session を消す）は apps/api-go/admin_users_db_test.go が本物の DB で確かめる。

const emails: string[] = [];
const password = "correct-horse-battery-staple";

afterAll(async () => {
  if (emails.length > 0) await execute("DELETE FROM `user` WHERE email IN (?)", [emails]);
  await pool.end();
});

/** 登録してメール確認を済ませた利用者を作る。 */
async function verifiedUser() {
  const email = `banned-${randomUUID()}@example.test`;
  emails.push(email);
  const { user } = await auth.api.signUpEmail({ body: { email, password, name: "x" } });
  await execute("UPDATE `user` SET emailVerified = true WHERE email = ?", [email]);
  return { id: user.id, email };
}

async function sessionCount(userId: string) {
  const [rows] = await pool.query("SELECT COUNT(*) AS n FROM session WHERE userId = ?", [userId]);
  return Number((rows as { n: number }[])[0].n);
}

describe("停止された利用者のログイン", () => {
  it("bannedAt があればセッションを作らず、停止の文言で断る。解除すればまたログインできる", async () => {
    const { id, email } = await verifiedUser();
    await auth.api.signInEmail({ body: { email, password } });
    const before = await sessionCount(id);
    expect(before).toBeGreaterThan(0);

    // Go の停止（sqlAdminUserStore.ban）と同じ書き方。日時は UTC で入れる。
    await execute("UPDATE `user` SET bannedAt = COALESCE(bannedAt, ?), updatedAt = ? WHERE id = ?", [
      new Date(),
      new Date(),
      id,
    ]);
    await expect(auth.api.signInEmail({ body: { email, password } })).rejects.toMatchObject({
      statusCode: 403,
      body: { message: "このアカウントは利用を停止されています。" },
    });
    expect(await sessionCount(id)).toBe(before);

    // Go の停止解除（sqlAdminUserStore.unban）と同じ書き方。
    await execute("UPDATE `user` SET bannedAt = NULL, updatedAt = ? WHERE id = ?", [new Date(), id]);
    await auth.api.signInEmail({ body: { email, password } });
    expect(await sessionCount(id)).toBe(before + 1);
  });
});
