import { createHash, randomUUID } from "node:crypto";
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";

// 確認メール・登録通知は送らない。Better Auth 本体は本物を通す（応答の形はここで決まるため）。
vi.mock("@/api/infra/email", () => ({
  notifyAdminOfNewUser: vi.fn(),
  sendVerificationEmail: vi.fn(),
  sendPasswordResetEmail: vi.fn(),
  sendPasswordChangedNotice: vi.fn(),
}));

const { auth } = await import("./auth.ts");
const { execute, pool, select } = await import("@/api/infra/db");
const { sendPasswordResetEmail, sendVerificationEmail } = await import("@/api/infra/email");
const { SIGN_IN_MAX_ATTEMPTS, SIGN_IN_WINDOW_MS } = await import("./sign-in-throttle.ts");
const { DEMO_EMAIL } = await import("@/shared/demo");

// 総当たりと列挙（セキュリティ基準 06 の B4）。
//   - 同じアカウントへのサインインは、IP を変えても上限で止まる（IP 単位の制限は Better Auth の rateLimit）
//   - 存在するメールと存在しないメールで、サインイン・登録・パスワード再設定の応答が同じ
// 上限で止まるのは本番の Better Auth の IP 単位の制限より後なので、ここでは本番だけ有効な
// その制限（NODE_ENV=production）は効いていない。

const emails: string[] = [];
const password = "correct-horse-battery-staple";
const wrongPassword = "wrong-password-123";

afterAll(async () => {
  if (emails.length > 0) {
    await execute("DELETE FROM `user` WHERE email IN (?)", [emails]);
  }
  await pool.end();
});

beforeEach(() => {
  vi.mocked(sendPasswordResetEmail).mockReset();
  vi.mocked(sendVerificationEmail).mockReset();
});

function newEmail() {
  const email = `sign-in-throttle-${randomUUID()}@example.test`;
  emails.push(email);
  return email;
}

/** 登録してメール確認を済ませた利用者を作る。 */
async function verifiedUser() {
  const email = newEmail();
  await auth.api.signUpEmail({ body: { email, password, name: "x" } });
  await execute("UPDATE `user` SET emailVerified = true WHERE email = ?", [email]);
  return email;
}

/**
 * 本番と同じく HTTP として Better Auth に渡し、状態コードと本文を返す。
 * auth.api を直接呼ぶと、フックで投げたエラーは応答にならずに例外のまま出てくる。
 * 1回ごとに別の IP から来たことにする（本番では nginx が X-Forwarded-For を付け直す）。
 */
type AuthResponseBody = {
  code?: string;
  message?: string;
  status?: boolean;
  token?: string | null;
  user?: Record<string, unknown>;
};

let ipSeq = 0;
async function post(path: string, body: Record<string, unknown>) {
  ipSeq += 1;
  const res = await auth.handler(
    new Request(`http://localhost:4000/api/auth${path}`, {
      method: "POST",
      headers: { "content-type": "application/json", "x-forwarded-for": `203.0.113.${ipSeq % 250}` },
      body: JSON.stringify(body),
    })
  );
  const json = (await res.json()) as AuthResponseBody;
  return { status: res.status, body: json, retryAfter: res.headers.get("x-retry-after") };
}

function signIn(email: string, pass: string) {
  return post("/sign-in/email", { email, password: pass });
}

async function failTimes(email: string, times: number) {
  for (let i = 0; i < times; i += 1) {
    expect((await signIn(email, wrongPassword)).status).toBe(401);
  }
}

describe("B4 アカウント単位の回数制限", () => {
  it("IP を変えても、同じアカウントへの試行は上限を超えると 429 になり、正しいパスワードでも入れない", async () => {
    const email = await verifiedUser();
    await failTimes(email, SIGN_IN_MAX_ATTEMPTS);

    const blocked = await signIn(email, password);
    expect(blocked.status).toBe(429);
    expect(blocked.body.code).toBe("TOO_MANY_SIGN_IN_ATTEMPTS");
    expect(Number(blocked.retryAfter)).toBeGreaterThan(0);
    expect(Number(blocked.retryAfter)).toBeLessThanOrEqual(SIGN_IN_WINDOW_MS / 1000);
  });

  it("大文字にしても同じアカウントとして数える", async () => {
    const email = await verifiedUser();
    await failTimes(email.toUpperCase(), SIGN_IN_MAX_ATTEMPTS);

    expect((await signIn(email, password)).status).toBe(429);
  });

  it("ほかのアカウントは止めない", async () => {
    const attacked = await verifiedUser();
    const other = await verifiedUser();
    await failTimes(attacked, SIGN_IN_MAX_ATTEMPTS);

    expect((await signIn(other, password)).status).toBe(200);
  });

  it("ログインできたら数え直す", async () => {
    const email = await verifiedUser();
    await failTimes(email, SIGN_IN_MAX_ATTEMPTS - 1);
    expect((await signIn(email, password)).status).toBe(200);

    await failTimes(email, SIGN_IN_MAX_ATTEMPTS - 1);
    expect((await signIn(email, password)).status).toBe(200);
  });

  it("窓が終われば、また試せる", async () => {
    const email = await verifiedUser();
    await failTimes(email, SIGN_IN_MAX_ATTEMPTS);
    expect((await signIn(email, password)).status).toBe(429);

    // 窓の始まりを、窓の長さより前にずらす。
    const emailHash = createHash("sha256").update(email.toLowerCase()).digest("hex");
    await execute("UPDATE SignInAttempt SET windowStartedAt = ? WHERE emailHash = ?", [
      new Date(Date.now() - SIGN_IN_WINDOW_MS - 1000),
      emailHash,
    ]);
    expect((await signIn(email, password)).status).toBe(200);
  });

  it("窓が終わったほかの行は、サインインを受けたときに消える", async () => {
    const staleHash = createHash("sha256").update(randomUUID()).digest("hex");
    await execute("INSERT INTO SignInAttempt (emailHash, count, windowStartedAt) VALUES (?, 3, ?)", [
      staleHash,
      new Date(Date.now() - SIGN_IN_WINDOW_MS - 1000),
    ]);

    await signIn(newEmail(), wrongPassword);

    const [{ n }] = await select<{ n: number }>(
      "SELECT COUNT(*) AS n FROM SignInAttempt WHERE emailHash = ?",
      [staleHash]
    );
    expect(n).toBe(0);
  });

  it("同時に投げられても、上限を超えた分は通さない", async () => {
    const email = await verifiedUser();
    const results = await Promise.all(
      Array.from({ length: SIGN_IN_MAX_ATTEMPTS + 5 }, () => signIn(email, wrongPassword))
    );

    expect(results.filter((r) => r.status === 401)).toHaveLength(SIGN_IN_MAX_ATTEMPTS);
    expect(results.filter((r) => r.status === 429)).toHaveLength(5);
  });

  it("デモアカウントは数えない（パスワードを公開しており、わざと失敗させて締め出せてしまうため）", async () => {
    for (let i = 0; i < SIGN_IN_MAX_ATTEMPTS + 1; i += 1) {
      expect((await signIn(DEMO_EMAIL, wrongPassword)).status).toBe(401);
    }
  });
});

describe("B4 メールアドレスの有無で応答が変わらない", () => {
  it("サインイン：パスワード違いと存在しないメールで、状態コードも本文も同じ", async () => {
    const existing = await verifiedUser();
    const missing = newEmail();

    const a = await signIn(existing, wrongPassword);
    const b = await signIn(missing, wrongPassword);
    expect(a.status).toBe(401);
    expect(b).toEqual(a);
  });

  it("サインイン：止まる回数も止まったときの応答も同じ", async () => {
    const existing = await verifiedUser();
    const missing = newEmail();
    await failTimes(existing, SIGN_IN_MAX_ATTEMPTS);
    await failTimes(missing, SIGN_IN_MAX_ATTEMPTS);

    const a = await signIn(existing, wrongPassword);
    const b = await signIn(missing, wrongPassword);
    expect(a.status).toBe(429);
    expect({ ...b, retryAfter: null }).toEqual({ ...a, retryAfter: null });
  });

  it("登録：登録済みのメールでも、新しいメールと同じ形で 200 を返す", async () => {
    const existing = await verifiedUser();
    const missing = newEmail();

    const register = async (email: string) => {
      const { status, body } = await post("/sign-up/email", { email, password, name: "x" });
      return { status, token: body.token, user: body.user ?? ({} as Record<string, unknown>) };
    };
    const a = await register(existing);
    const b = await register(missing);

    expect(a.status).toBe(200);
    expect(b.status).toBe(200);
    expect(a.token).toBeNull();
    expect(b.token).toBeNull();
    // id と日時は毎回違うので、項目の並びと、変わらないはずの値だけを比べる。
    expect(Object.keys(a.user).sort()).toEqual(Object.keys(b.user).sort());
    const stable = ({ id: _id, email: _email, createdAt: _c, updatedAt: _u, ...rest }: Record<string, unknown>) => rest;
    expect(stable(a.user)).toEqual(stable(b.user));
  });

  it("再設定：登録済みのメールと存在しないメールで、状態コードも本文も同じ", async () => {
    const existing = await verifiedUser();
    const missing = newEmail();

    const requestReset = async (email: string) => {
      const { status, body } = await post("/request-password-reset", { email });
      return { status, body };
    };
    const a = await requestReset(existing);
    const b = await requestReset(missing);

    expect(a.status).toBe(200);
    expect(b).toEqual(a);
    expect(sendPasswordResetEmail).toHaveBeenCalledOnce();
  });

  it("再設定・登録：メールを送り終えるのを待たずに返す（応答時間で登録の有無を分からなくする）", async () => {
    const existing = await verifiedUser();
    vi.mocked(sendVerificationEmail).mockClear();
    // 送信が終わらないメール。応答がこれを待つなら、テストは制限時間で落ちる。
    vi.mocked(sendPasswordResetEmail).mockReturnValue(new Promise(() => {}));
    vi.mocked(sendVerificationEmail).mockReturnValue(new Promise(() => {}));

    await expect(post("/request-password-reset", { email: existing })).resolves.toMatchObject({
      status: 200,
    });
    expect(sendPasswordResetEmail).toHaveBeenCalledOnce();

    await expect(post("/sign-up/email", { email: newEmail(), password, name: "x" })).resolves.toMatchObject({
      status: 200,
    });
    expect(sendVerificationEmail).toHaveBeenCalledOnce();
  });
});
