import { randomUUID } from "node:crypto";
import { afterAll, describe, expect, it, vi } from "vitest";

// 確認メール・登録通知は送らない。Better Auth 本体は本物を通す（穴はここで決まるため）。
vi.mock("@/api/infra/email", () => ({
  notifyAdminOfNewUser: vi.fn(),
  sendVerificationEmail: vi.fn(),
  sendPasswordResetEmail: vi.fn(),
  sendPasswordChangedNotice: vi.fn(),
}));

const { auth } = await import("./auth.ts");
const { execute, pool, select } = await import("@/api/infra/db");

// 利用者が変えてはいけない項目を、Better Auth の登録（sign-up）と更新（update-user）の
// 本文に混ぜても書き換わらないことを確かめる（セキュリティ基準 06 の A4・B7）。
// 自前の API 側は forbidden-fields.test.ts で見る。
//
// role・bannedAt は auth.ts の additionalFields で input: false にしてある。これが外れると
// 本文に role: "admin" を混ぜるだけで管理者になれ、bannedAt: null で停止を解ける。
// emailVerified を登録で true にできると、メール確認を飛ばせる。
// 設定の見た目ではなく、実際に Better Auth の API を通して確かめる。
// 項目ごとに分けて送るのは、1つが 400 で拒まれると、残りを確かめられないため。

const emails: string[] = [];
const password = "correct-horse-battery-staple";

type Protected = { role: string; bannedAt: Date | null; emailVerified: boolean; email: string };

async function protectedColumnsOf(email: string) {
  const [row] = await select<Protected>(
    "SELECT role, bannedAt, emailVerified, email FROM `user` WHERE email = ?",
    [email]
  );
  return row;
}

async function protectedColumnsById(id: string) {
  const [row] = await select<Protected>(
    "SELECT role, bannedAt, emailVerified, email FROM `user` WHERE id = ?",
    [id]
  );
  return row;
}

// 失敗しても成功しても良い。見るのは「DB の値が変わっていないこと」だけ。
async function attempt(fn: () => Promise<unknown>) {
  try {
    await fn();
  } catch {
    // Better Auth は input: false の項目を送ると 400 で拒む
  }
}

function newEmail() {
  const email = `forbidden-${randomUUID()}@example.test`;
  emails.push(email);
  return email;
}

afterAll(async () => {
  if (emails.length > 0) await execute("DELETE FROM `user` WHERE email IN (?)", [emails]);
  await pool.end();
});

const SIGN_UP_FIELDS: [string, Record<string, unknown>][] = [
  ["role", { role: "admin" }],
  ["bannedAt", { bannedAt: new Date("2000-01-01T00:00:00.000Z") }],
  ["emailVerified", { emailVerified: true }],
];

describe("sign-up の本文に禁止項目を混ぜても、既定値のまま", () => {
  it.each(SIGN_UP_FIELDS)("%s", async (_field, forbidden) => {
    const email = newEmail();

    await attempt(() =>
      auth.api.signUpEmail({ body: { email, password, name: "x", ...forbidden } as never })
    );
    // 400 で拒まれて行ができない場合もあるので、そのときは項目を付けずに登録し直し、
    // 既定値を確かめる。
    if ((await protectedColumnsOf(email)) === undefined) {
      await auth.api.signUpEmail({ body: { email, password, name: "x" } });
    }
    expect(await protectedColumnsOf(email)).toEqual({
      role: "user",
      bannedAt: null,
      emailVerified: false,
      email,
    });
  });
});

const UPDATE_USER_FIELDS: [string, Record<string, unknown>][] = [
  ["role", { role: "admin" }],
  ["bannedAt", { bannedAt: new Date("2000-01-01T00:00:00.000Z") }],
  ["emailVerified", { emailVerified: false }],
  ["email", { email: "taken-over@example.test" }],
  ["id", { id: `taken-over-${randomUUID()}` }],
];

/** 登録してメール確認を済ませ、ログインした Cookie を返す。 */
async function signedInUser() {
  const email = newEmail();
  const { user } = await auth.api.signUpEmail({ body: { email, password, name: "x" } });
  await execute("UPDATE `user` SET emailVerified = true WHERE email = ?", [email]);
  const { headers } = await auth.api.signInEmail({ body: { email, password }, returnHeaders: true });
  const cookie = headers.getSetCookie().map((value) => value.split(";")[0]).join("; ");
  expect(cookie).not.toBe("");
  return { id: user.id, cookie };
}

describe("update-user の本文に禁止項目を混ぜても、変わらない", () => {
  it.each(UPDATE_USER_FIELDS)("%s", async (_field, forbidden) => {
    const { id, cookie } = await signedInUser();
    const before = await protectedColumnsById(id);
    expect(before).toMatchObject({ role: "user", bannedAt: null, emailVerified: true });

    await attempt(() =>
      auth.api.updateUser({
        body: { name: "y", ...forbidden } as never,
        headers: new Headers({ cookie }),
      })
    );

    expect(await protectedColumnsById(id)).toEqual(before);
  });
});
