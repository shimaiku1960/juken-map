import { randomUUID } from "node:crypto";
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";

// 確認メール・登録通知は送らない。再設定のメールは URL（トークン）を取り出すために捕まえる。
// Better Auth 本体は本物を通す（セッションを消すかどうかはここで決まるため）。
vi.mock("@/api/infra/email", () => ({
  notifyAdminOfNewUser: vi.fn(),
  sendVerificationEmail: vi.fn(),
  sendPasswordResetEmail: vi.fn(),
  sendPasswordChangedNotice: vi.fn(),
}));

const { auth } = await import("./auth.ts");
const { execute, pool } = await import("@/api/infra/db");
const { sendPasswordChangedNotice, sendPasswordResetEmail } = await import("@/api/infra/email");
const { buildTestApp, request } = await import("./test-support.ts");

// 乗っ取り後の封じ込め（セキュリティ基準 06 の B6）。
//   - パスワードの変更には今のパスワードがいる。メールアドレスは変えられない
//   - 変更・再設定のあと、ほかのセッションは 401 になる（本文で revokeOtherSessions: false と送っても）
//   - 変更・再設定したら本人のメールアドレスへ知らせる
// 利用停止でセッションが消えることは、停止を受け持つ Go の admin_users_db_test.go が確かめる。

const emails: string[] = [];
const password = "correct-horse-battery-staple";
const newPassword = "new-correct-horse-battery";
// ログインが要るルートなら何でもよい。業務の API は Go へ移っていくので、ここで1本だけ用意する。
const app = buildTestApp((app) => {
  app.get("/test/session", { config: { access: "user" } }, async () => ({ ok: true }));
});

afterAll(async () => {
  if (emails.length > 0) await execute("DELETE FROM `user` WHERE email IN (?)", [emails]);
  await app.close();
  await pool.end();
});

beforeEach(() => {
  vi.mocked(sendPasswordChangedNotice).mockClear();
});

/** Set-Cookie から、次のリクエストに付ける Cookie ヘッダーを作る。 */
function cookieFrom(headers: Headers) {
  return headers
    .getSetCookie()
    .map((value) => value.split(";")[0])
    .filter((pair) => !pair.endsWith("="))
    .join("; ");
}

/** 登録してメール確認を済ませた利用者を作る。 */
async function verifiedUser() {
  const email = `password-change-${randomUUID()}@example.test`;
  emails.push(email);
  await auth.api.signUpEmail({ body: { email, password, name: "x" } });
  await execute("UPDATE `user` SET emailVerified = true WHERE email = ?", [email]);
  return email;
}

/** ログインして、そのセッションの Cookie を返す（呼ぶたびに別の端末のセッションになる）。 */
async function signIn(email: string, pass = password) {
  const { headers } = await auth.api.signInEmail({ body: { email, password: pass }, returnHeaders: true });
  return cookieFrom(headers);
}

/** ログインが要るルートを、その Cookie で叩いた結果の状態コード。 */
async function statusWith(cookie: string) {
  const res = await request(app, "GET", "/test/session", undefined, { cookie });
  return res.statusCode;
}

describe("B6 パスワードの変更", () => {
  it("変更すると、本文で revokeOtherSessions: false と送ってもほかのセッションは 401 になり、変更した端末は使える", async () => {
    const email = await verifiedUser();
    const other = await signIn(email);
    const current = await signIn(email);
    expect(await statusWith(other)).toBe(200);

    const { headers } = await auth.api.changePassword({
      body: { currentPassword: password, newPassword, revokeOtherSessions: false },
      headers: new Headers({ cookie: current }),
      returnHeaders: true,
    });

    expect(await statusWith(other)).toBe(401);
    expect(await statusWith(current)).toBe(401);
    // 今の端末には新しいセッションが発行される（Set-Cookie で差し替わる）。
    expect(await statusWith(cookieFrom(headers))).toBe(200);
    expect(sendPasswordChangedNotice).toHaveBeenCalledExactlyOnceWith(email);
  });

  it("今のパスワードが違えば変更せず、セッションも消さず、知らせもしない", async () => {
    const email = await verifiedUser();
    const other = await signIn(email);
    const current = await signIn(email);

    await expect(
      auth.api.changePassword({
        body: { currentPassword: "wrong-password-123", newPassword },
        headers: new Headers({ cookie: current }),
      })
    ).rejects.toMatchObject({ statusCode: 400, body: { code: "INVALID_PASSWORD" } });

    expect(await statusWith(other)).toBe(200);
    expect(await statusWith(current)).toBe(200);
    await expect(signIn(email, newPassword)).rejects.toMatchObject({ statusCode: 401 });
    expect(sendPasswordChangedNotice).not.toHaveBeenCalled();
  });

  it("今のパスワードを付けずに変更することはできない", async () => {
    const email = await verifiedUser();
    const current = await signIn(email);

    await expect(
      auth.api.changePassword({
        // 型の上では必須なので、外から本文を省いて送られた場合を作る。
        body: { newPassword } as { newPassword: string; currentPassword: string },
        headers: new Headers({ cookie: current }),
      })
    ).rejects.toMatchObject({ statusCode: 400 });
    await expect(signIn(email, newPassword)).rejects.toMatchObject({ statusCode: 401 });
  });
});

describe("B6 メールアドレスの変更", () => {
  it("変更の API は無効で、プロフィールの更新にメールアドレスを混ぜても変わらない", async () => {
    const email = await verifiedUser();
    const current = await signIn(email);
    const headers = new Headers({ cookie: current });

    await expect(
      auth.api.changeEmail({ body: { newEmail: `taken-over-${randomUUID()}@example.test` }, headers })
    ).rejects.toMatchObject({ statusCode: 400, body: { code: "CHANGE_EMAIL_DISABLED" } });
    await expect(
      // 型の上では email を受け付けないので、外から本文に混ぜて送られた場合を作る。
      auth.api.updateUser({ body: { email: `taken-over-${randomUUID()}@example.test` } as unknown as { name: string }, headers })
    ).rejects.toMatchObject({ statusCode: 400, body: { code: "EMAIL_CAN_NOT_BE_UPDATED" } });

    expect((await auth.api.getSession({ headers }))?.user.email).toBe(email);
  });
});

describe("B6 パスワードの再設定", () => {
  it("再設定すると、その人のセッションはすべて 401 になり、本人に知らせる", async () => {
    const email = await verifiedUser();
    const sessions = [await signIn(email), await signIn(email)];
    for (const cookie of sessions) expect(await statusWith(cookie)).toBe(200);

    await auth.api.requestPasswordReset({ body: { email, redirectTo: "/reset-password" } });
    // テストでは baseURL が無いので、URL は「…/reset-password/<トークン>?callbackURL=…」の相対の形になる。
    const url = vi.mocked(sendPasswordResetEmail).mock.lastCall?.[1] ?? "";
    const token = /\/reset-password\/([^/?]+)/.exec(url)?.[1] ?? "";
    await auth.api.resetPassword({ body: { newPassword, token } });

    for (const cookie of sessions) expect(await statusWith(cookie)).toBe(401);
    expect(await statusWith(await signIn(email, newPassword))).toBe(200);
    expect(sendPasswordChangedNotice).toHaveBeenCalledExactlyOnceWith(email);
  });
});
