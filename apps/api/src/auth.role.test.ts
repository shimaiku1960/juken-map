import { randomUUID } from "node:crypto";
import { afterAll, describe, expect, it, vi } from "vitest";

// 確認メール・登録通知は送らない。Better Auth 本体は本物を通す（role の穴はここで決まるため）。
vi.mock("@/api/infra/email", () => ({
  notifyAdminOfNewUser: vi.fn(),
  sendVerificationEmail: vi.fn(),
  sendPasswordResetEmail: vi.fn(),
}));

const { auth } = await import("./auth.ts");
const { execute, pool, select } = await import("@/api/infra/db");

// role は auth.ts の additionalFields で input: false にしてある。
// これが外れると、登録やプロフィール更新の本文に role: "admin" を混ぜるだけで管理者になれる。
// 設定の見た目ではなく、実際に Better Auth の API を通して確かめる。

const emails: string[] = [];
const password = "correct-horse-battery-staple";

async function roleOf(email: string) {
  const [row] = await select<{ role: string }>("SELECT role FROM `user` WHERE email = ?", [email]);
  return row?.role;
}

// 失敗しても成功しても良い。見るのは「DB の role が変わっていないこと」だけ。
async function attempt(fn: () => Promise<unknown>) {
  try {
    await fn();
  } catch {
    // Better Auth は input: false の項目を送ると 400 で拒む
  }
}

afterAll(async () => {
  if (emails.length > 0) await execute("DELETE FROM `user` WHERE email IN (?)", [emails]);
  await pool.end();
});

describe("role は利用者が自分で書き換えられない", () => {
  it("サインアップの本文に role: admin を混ぜても user のまま", async () => {
    const email = `role-${randomUUID()}@example.test`;
    emails.push(email);

    await attempt(() =>
      auth.api.signUpEmail({
        body: { email, password, name: "x", role: "admin" } as never,
      })
    );
    // 400 で拒まれて行ができない場合もあるので、できたときだけ確かめる。そのうえで
    // role を付けずに登録し直し、既定値が user であることも確かめる。
    if ((await roleOf(email)) === undefined) {
      await auth.api.signUpEmail({ body: { email, password, name: "x" } });
    }
    expect(await roleOf(email)).toBe("user");
  });

  it("update-user の本文に role: admin を混ぜても user のまま", async () => {
    const email = `role-${randomUUID()}@example.test`;
    emails.push(email);
    await auth.api.signUpEmail({ body: { email, password, name: "x" } });
    await execute("UPDATE `user` SET emailVerified = true WHERE email = ?", [email]);

    const { headers } = await auth.api.signInEmail({
      body: { email, password },
      returnHeaders: true,
    });
    const cookie = headers.getSetCookie().map((value) => value.split(";")[0]).join("; ");
    expect(cookie).not.toBe("");

    await attempt(() =>
      auth.api.updateUser({
        body: { role: "admin" } as never,
        headers: new Headers({ cookie }),
      })
    );

    expect(await roleOf(email)).toBe("user");
  });
});
