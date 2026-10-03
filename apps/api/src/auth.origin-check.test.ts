import { randomUUID } from "node:crypto";
import { afterAll, describe, expect, it, vi } from "vitest";

// 確認メール・登録通知・変更の知らせは送らない。Better Auth 本体は本物を通す（オリジンを確かめるのは Better Auth）。
vi.mock("@/api/infra/email", () => ({
  notifyAdminOfNewUser: vi.fn(),
  sendVerificationEmail: vi.fn(),
  sendPasswordResetEmail: vi.fn(),
  sendPasswordChangedNotice: vi.fn(),
}));

const { auth } = await import("./auth.ts");
const { execute, pool } = await import("@/api/infra/db");

// CSRF 対策（セキュリティ基準 06 の D3）のうち、Node に残るログインまわり（/api/auth/*）。
//   - Cookie 付きの書き込みは、Origin が trustedOrigins（auth.ts）に無ければ 403。Origin も Referer も無くても 403
//   - セッションの Cookie は SameSite=Lax・HttpOnly（別のサイトからの POST には Cookie そのものが付かない）
// 業務の API（Go）の側は apps/api-go の TestRegisteredWritesRejectCrossSite と TestRouterCrossOrigin が確かめる。
//
// 本番と同じく、Better Auth の handler に Request をそのまま渡す（server.ts は toNodeHandler で包んで渡している）。

const base = "http://localhost:4000/api/auth";
const trusted = "https://juken-map.com";
const password = "correct-horse-battery-staple";
const emails: string[] = [];

afterAll(async () => {
  if (emails.length > 0) await execute("DELETE FROM `user` WHERE email IN (?)", [emails]);
  await pool.end();
});

/** 登録してメール確認を済ませ、ログインした利用者のメールアドレスと Cookie を返す。 */
async function signedIn() {
  const email = `origin-check-${randomUUID()}@example.test`;
  emails.push(email);
  await auth.api.signUpEmail({ body: { email, password, name: "x" } });
  await execute("UPDATE `user` SET emailVerified = true WHERE email = ?", [email]);
  const res = await send("POST", "/sign-in/email", { origin: trusted }, { email, password });
  expect(res.status).toBe(200);
  const cookie = res.headers
    .getSetCookie()
    .map((value) => value.split(";")[0])
    .join("; ");
  return { email, cookie, setCookie: res.headers.getSetCookie() };
}

async function send(method: string, path: string, headers: Record<string, string>, body?: unknown) {
  return auth.handler(
    new Request(base + path, {
      method,
      headers: body === undefined ? headers : { "content-type": "application/json", ...headers },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  );
}

/** その Cookie のセッションがまだ使えるか。 */
async function sessionAlive(cookie: string) {
  const res = await send("GET", "/get-session", { cookie });
  return (await res.json()) !== null;
}

describe("D3 /api/auth/* の書き込みは、別のサイトからだと断る", () => {
  for (const [name, headers] of [
    ["別のサイトの Origin", { origin: "https://evil.example" }],
    ["似ているが別のホスト", { origin: "https://juken-map.com.evil.example" }],
    ["Origin も Referer も無い", {}],
    ["別のサイトの Referer", { referer: "https://evil.example/page" }],
  ] as const) {
    it(`ログアウト（${name}）は 403 で、セッションは消えない`, async () => {
      const { cookie } = await signedIn();
      const res = await send("POST", "/sign-out", { cookie, ...headers }, {});
      expect(res.status).toBe(403);
      expect(await sessionAlive(cookie)).toBe(true);
    });
  }

  it("パスワードの変更（別のサイトの Origin）は 403 で、パスワードは変わらない", async () => {
    const { email, cookie } = await signedIn();
    const res = await send(
      "POST",
      "/change-password",
      { cookie, origin: "https://evil.example" },
      { currentPassword: password, newPassword: "attacker-chosen-password" }
    );
    expect(res.status).toBe(403);
    const again = await send("POST", "/sign-in/email", { origin: trusted }, { email, password });
    expect(again.status).toBe(200);
  });

  it("同じサイトの画面（trustedOrigins）からなら通る", async () => {
    const { cookie } = await signedIn();
    const res = await send("POST", "/sign-out", { cookie, origin: trusted }, {});
    expect(res.status).toBe(200);
    expect(await sessionAlive(cookie)).toBe(false);
  });
});

describe("D3 セッションの Cookie", () => {
  it("SameSite=Lax・HttpOnly で発行される", async () => {
    const { setCookie } = await signedIn();
    const session = setCookie.find((value) => value.includes("session_token="));
    expect(session).toBeDefined();
    expect(session).toMatch(/;\s*SameSite=Lax/i);
    expect(session).toMatch(/;\s*HttpOnly/i);
  });
});
