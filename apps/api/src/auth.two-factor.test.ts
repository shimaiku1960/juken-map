import { randomUUID } from "node:crypto";
import { afterAll, describe, expect, it, vi } from "vitest";

// 確認メール・登録通知は送らない。Better Auth 本体は本物を通す（印の付き方はここで決まるため）。
vi.mock("@/api/infra/email", () => ({
  notifyAdminOfNewUser: vi.fn(),
  sendVerificationEmail: vi.fn(),
  sendPasswordResetEmail: vi.fn(),
  sendPasswordChangedNotice: vi.fn(),
}));

// microCMS は読み込むだけで API キーを要求するので差し替える（管理 API の登録で読まれる）。
vi.mock("@/api/infra/microcms", () => ({
  getBlog: vi.fn(),
  listBlogs: vi.fn(),
  isBlogNotFound: vi.fn(() => false),
}));

const { auth } = await import("./auth.ts");
const { execute, pool } = await import("@/api/infra/db");
const { registerAdminRoutes } = await import("./routes/admin.ts");
const { buildTestApp, request } = await import("./test-support.ts");
const { TWO_FACTOR_REQUIRED } = await import("@/shared/admin");

// 管理者の2段階認証（セキュリティ基準 06 の B7）。認証は差し替えず、実際の Better Auth で
//   ログイン → 2段階認証を有効にしてコードを確かめる → ログインし直して予備コードを確かめる
// と進め、印（session.twoFactorVerified）がコードを確かめたセッションにだけ付くこと、
// 管理 API がその印で 403 と 200 に分かれることを確かめる。

const emails: string[] = [];
const password = "correct-horse-battery-staple";
const app = buildTestApp(registerAdminRoutes);

afterAll(async () => {
  if (emails.length > 0) await execute("DELETE FROM `user` WHERE email IN (?)", [emails]);
  await app.close();
  await pool.end();
});

/** Set-Cookie から、次のリクエストに付ける Cookie ヘッダーを作る。 */
function cookieFrom(headers: Headers) {
  return headers
    .getSetCookie()
    .map((value) => value.split(";")[0])
    .filter((pair) => !pair.endsWith("="))
    .join("; ");
}

/** otpauth:// の secret（RFC 4648 の base32）を、Better Auth が持つ元の文字列に戻す。 */
function secretFromTotpURI(uri: string) {
  const encoded = new URL(uri).searchParams.get("secret") ?? "";
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const char of encoded) bits += alphabet.indexOf(char).toString(2).padStart(5, "0");
  const bytes = bits.match(/.{8}/g)?.map((byte) => parseInt(byte, 2)) ?? [];
  return Buffer.from(bytes).toString("utf8");
}

async function sessionOf(cookie: string) {
  return auth.api.getSession({ headers: new Headers({ cookie }) });
}

/** 登録してメール確認を済ませ、管理者にしてからメール＋パスワードでログインする。 */
async function signedInAdmin() {
  const email = `two-factor-${randomUUID()}@example.test`;
  emails.push(email);
  await auth.api.signUpEmail({ body: { email, password, name: "x" } });
  await execute("UPDATE `user` SET emailVerified = true, role = 'admin' WHERE email = ?", [email]);
  const { headers } = await auth.api.signInEmail({ body: { email, password }, returnHeaders: true });
  return { email, cookie: cookieFrom(headers) };
}

const overview = (cookie: string) =>
  request(app, "GET", "/api/admin/overview", undefined, { cookie });

describe("B7 管理者の2段階認証（実際の Better Auth）", () => {
  it("有効化の前のメール＋パスワードのログインは印が無く、管理 API は 403", async () => {
    const { cookie } = await signedInAdmin();

    expect((await sessionOf(cookie))?.session.twoFactorVerified).toBe(false);
    const res = await overview(cookie);
    expect(res.statusCode).toBe(403);
    expect(res.json()).toMatchObject({ code: TWO_FACTOR_REQUIRED });
  });

  it("有効化してコードを確かめると印の付いたセッションに替わり、次のログインはコード無しでは作られない", async () => {
    const { email, cookie } = await signedInAdmin();

    // 有効化にはパスワードが要る（乗っ取ったセッションだけでは有効化・無効化できない）。
    await expect(
      auth.api.enableTwoFactor({ body: { password: "wrong-password" }, headers: new Headers({ cookie }) })
    ).rejects.toThrow();
    const { totpURI, backupCodes } = await auth.api.enableTwoFactor({
      body: { password },
      headers: new Headers({ cookie }),
    });
    const { code } = await auth.api.generateTOTP({ body: { secret: secretFromTotpURI(totpURI) } });

    const verified = await auth.api.verifyTOTP({
      body: { code },
      headers: new Headers({ cookie }),
      returnHeaders: true,
    });
    const verifiedCookie = cookieFrom(verified.headers);
    const session = await sessionOf(verifiedCookie);
    expect(session?.session.twoFactorVerified).toBe(true);
    expect(session?.user.twoFactorEnabled).toBe(true);
    expect((await overview(verifiedCookie)).statusCode).toBe(200);
    // 有効化の前のセッションは消えている
    expect(await sessionOf(cookie)).toBeNull();

    // 有効にしたあとのメール＋パスワードのログインは、セッションを作らずコードを求める。
    const signIn = await auth.api.signInEmail({ body: { email, password }, returnHeaders: true });
    expect(signIn.response).toMatchObject({ twoFactorRedirect: true });
    const pendingCookie = cookieFrom(signIn.headers);
    expect(await sessionOf(pendingCookie)).toBeNull();

    // 予備コードでも通れ、そのセッションには印が付く。
    const byBackup = await auth.api.verifyBackupCode({
      body: { code: backupCodes[0] },
      headers: new Headers({ cookie: pendingCookie }),
      returnHeaders: true,
    });
    const backupCookie = cookieFrom(byBackup.headers);
    expect((await sessionOf(backupCookie))?.session.twoFactorVerified).toBe(true);
    expect((await overview(backupCookie)).statusCode).toBe(200);
  });

  it("コードが違えば、セッションは作られない", async () => {
    const { email, cookie } = await signedInAdmin();
    await auth.api.enableTwoFactor({ body: { password }, headers: new Headers({ cookie }) });
    // 有効化の確認をしないと twoFactorEnabled は立たないので、ここでは直接立てる。
    await execute("UPDATE `user` SET twoFactorEnabled = true WHERE email = ?", [email]);

    const signIn = await auth.api.signInEmail({ body: { email, password }, returnHeaders: true });
    const pendingCookie = cookieFrom(signIn.headers);
    await expect(
      auth.api.verifyTOTP({ body: { code: "000000" }, headers: new Headers({ cookie: pendingCookie }) })
    ).rejects.toThrow();
    expect(await sessionOf(pendingCookie)).toBeNull();
  });
});
