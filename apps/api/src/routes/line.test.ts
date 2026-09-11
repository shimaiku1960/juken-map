import { afterAll, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

// 差し替えるのは外部境界（認証・LINE の API）だけ。DB は本物のテスト用 MySQL に流す。
vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

vi.mock("@/api/infra/line", () => ({
  verifyLineSignature: vi.fn(),
  issueLineLinkToken: vi.fn(),
  lineAccountLinkUrl: vi.fn(
    () => "https://juken-map.com/line/link?linkToken=token"
  ),
  replyLineText: vi.fn(),
  pushLineText: vi.fn(),
}));

vi.mock("@/api/infra/lineLogin", () => ({
  createLineOAuthValues: vi.fn(),
  lineLoginAuthorizationUrl: vi.fn(),
  exchangeLineLoginCode: vi.fn(),
  getLineFriendshipStatus: vi.fn(),
  verifyLineIdToken: vi.fn(),
}));

const { auth } = await import("../auth.ts");
const {
  verifyLineSignature,
  issueLineLinkToken,
  replyLineText,
  pushLineText,
} = await import("@/api/infra/line");
const {
  createLineOAuthValues,
  lineLoginAuthorizationUrl,
  exchangeLineLoginCode,
  getLineFriendshipStatus,
  verifyLineIdToken,
} = await import("@/api/infra/lineLogin");
const { registerLineRoutes } = await import("./line.ts");
const { buildTestApp, request, demoSession } = await import("../test-support.ts");
const {
  cleanup,
  createLineConnection,
  createLineLinkNonce,
  createLineOAuthAttempt,
  createNotificationPreference,
  createUser,
  findLineConnections,
  findLineLinkNonces,
  findLineOAuthAttempts,
  findNotificationPreferences,
} = await import("../test-db/fixtures.ts");

const getSession = auth.api.getSession as unknown as Mock;

const app = buildTestApp(registerLineRoutes);

// WEB_ORIGIN 未設定なので webOrigin() は SITE_URL を返す。
const ORIGIN = "https://juken-map.com";

const webhook = (events: unknown[]) =>
  request(app, "POST", "/api/line/webhook", { events }, {
    "x-line-signature": "signature",
  });

// LINE ユーザー ID は UNIQUE なので、テストごとに作る。
const newLineUserId = () => `U${crypto.randomUUID().replaceAll("-", "")}`;

let owner: Awaited<ReturnType<typeof createUser>>;

beforeEach(async () => {
  vi.clearAllMocks();
  owner = await createUser();
  getSession.mockResolvedValue(owner.session);
});

afterAll(cleanup);

describe("GET /line/settings", () => {
  it("ログイン済みならプロフィールの通知設定へ移動する", async () => {
    const res = await request(app, "GET", "/line/settings");

    expect(res.statusCode).toBe(302);
    expect(res.headers.location).toBe(
      `${ORIGIN}/profile#notification-settings`
    );
  });

  it("未ログインなら通知設定を戻り先にしてログインへ移動する", async () => {
    getSession.mockResolvedValue(null);

    const res = await request(app, "GET", "/line/settings");

    expect(res.statusCode).toBe(302);
    expect(res.headers.location).toBe(
      `${ORIGIN}/login?callbackURL=%2Fprofile%23notification-settings`
    );
  });
});

describe("POST /api/line/webhook", () => {
  const accountLink = (nonce: string, lineUserId: string) =>
    webhook([
      {
        type: "accountLink",
        replyToken: "reply-token",
        source: { type: "user", userId: lineUserId },
        link: { result: "ok", nonce },
      },
    ]);

  const linkMessage = (lineUserId: string) =>
    webhook([
      {
        type: "message",
        replyToken: "reply-token",
        source: { type: "user", userId: lineUserId },
        message: { type: "text", text: "連携" },
      },
    ]);

  beforeEach(() => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(replyLineText).mockResolvedValue(undefined);
  });

  it("署名が不正なら401を返す", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(false);

    expect((await webhook([])).statusCode).toBe(401);
  });

  it("連携メッセージへ公式Account Linking URLを返す", async () => {
    const lineUserId = newLineUserId();
    vi.mocked(issueLineLinkToken).mockResolvedValue("token");

    const res = await linkMessage(lineUserId);

    expect(res.statusCode).toBe(200);
    expect(issueLineLinkToken).toHaveBeenCalledWith(lineUserId);
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("10分以内")
    );
  });

  it("連携済みなら再連携リンクを発行せず通知設定を案内する", async () => {
    const lineUserId = await createLineConnection(owner.id);

    const res = await linkMessage(lineUserId);

    expect(res.statusCode).toBe(200);
    expect(issueLineLinkToken).not.toHaveBeenCalled();
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("すでに連携済み")
    );
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining(`${ORIGIN}/line/settings`)
    );
  });

  it("有効なnonceでアプリユーザーとLINEユーザーを連携し、nonceを使い捨てにする", async () => {
    const nonce = await createLineLinkNonce(owner.id);
    const lineUserId = newLineUserId();

    const res = await accountLink(nonce, lineUserId);

    expect(res.statusCode).toBe(200);
    expect(await findLineConnections(owner.id)).toEqual([
      expect.objectContaining({ lineUserId }),
    ]);
    expect(await findLineLinkNonces(owner.id)).toHaveLength(0);
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("連携が完了")
    );
  });

  it("連携済みのアカウントなら、新しいLINEへ付け替える", async () => {
    await createLineConnection(owner.id);
    const nonce = await createLineLinkNonce(owner.id);
    const lineUserId = newLineUserId();

    await accountLink(nonce, lineUserId);

    expect(await findLineConnections(owner.id)).toEqual([
      expect.objectContaining({ lineUserId }),
    ]);
  });

  it("別ユーザーに連携済みなら解除方法を返信し、どちらの連携も変えない", async () => {
    const other = await createUser();
    const lineUserId = await createLineConnection(other.id);
    const before = await findLineConnections(other.id);
    const nonce = await createLineLinkNonce(owner.id);

    const res = await accountLink(nonce, lineUserId);

    expect(res.statusCode).toBe(200);
    expect(await findLineConnections(owner.id)).toHaveLength(0);
    expect(await findLineConnections(other.id)).toEqual(before);
    // nonce は断ったときも使い捨て
    expect(await findLineLinkNonces(owner.id)).toHaveLength(0);
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("別の受験マップアカウント")
    );
  });

  it("期限切れのnonceなら連携せず、やり直しを案内する", async () => {
    const nonce = await createLineLinkNonce(owner.id, {
      expiresAt: new Date(Date.now() - 1_000),
    });

    await accountLink(nonce, newLineUserId());

    expect(await findLineConnections(owner.id)).toHaveLength(0);
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("期限が切れました")
    );
  });

  it("同じnonceが同時に2回届いても、連携は1回だけ行われる", async () => {
    const nonce = await createLineLinkNonce(owner.id);
    const lineUserId = newLineUserId();

    await Promise.all([accountLink(nonce, lineUserId), accountLink(nonce, lineUserId)]);

    expect(await findLineConnections(owner.id)).toHaveLength(1);
    const replies = vi.mocked(replyLineText).mock.calls.map(([, text]) => text);
    expect(replies.filter((text) => text.includes("連携が完了"))).toHaveLength(1);
    expect(replies.filter((text) => text.includes("期限が切れました"))).toHaveLength(1);
  });
});

describe("POST /api/line/account-link", () => {
  it("未ログインならnonceを発行しない", async () => {
    getSession.mockResolvedValue(null);

    const res = await request(app, "POST", "/api/line/account-link", {
      linkToken: "token",
    });

    expect(res.statusCode).toBe(401);
    expect(await findLineLinkNonces(owner.id)).toHaveLength(0);
  });

  it("ログインユーザーに10分間の単回nonceを発行する（古いnonceは捨てる）", async () => {
    await createLineLinkNonce(owner.id, { nonce: `old-${crypto.randomUUID()}` });
    const before = Date.now();

    const res = await request(app, "POST", "/api/line/account-link", {
      linkToken: "link-token",
    });

    expect(res.statusCode).toBe(200);
    const redirectUrl = new URL(res.json().redirectUrl);
    expect(redirectUrl.origin + redirectUrl.pathname).toBe(
      "https://access.line.me/dialog/bot/accountLink"
    );
    const nonces = await findLineLinkNonces(owner.id);
    expect(nonces).toHaveLength(1);
    expect(nonces[0].nonce).toBe(redirectUrl.searchParams.get("nonce"));
    expect(nonces[0].expiresAt.getTime()).toBeGreaterThanOrEqual(before + 9 * 60 * 1000);
    expect(nonces[0].expiresAt.getTime()).toBeLessThanOrEqual(Date.now() + 10 * 60 * 1000);
  });
});

describe("GET /api/line/connection", () => {
  it("連携の有無を返す", async () => {
    expect((await request(app, "GET", "/api/line/connection")).json()).toEqual({ connected: false });

    await createLineConnection(owner.id);

    expect((await request(app, "GET", "/api/line/connection")).json()).toEqual({ connected: true });
  });
});

describe("DELETE /api/line/connection", () => {
  it("未ログインなら連携を解除しない", async () => {
    await createLineConnection(owner.id);
    getSession.mockResolvedValue(null);

    const res = await request(app, "DELETE", "/api/line/connection");

    expect(res.statusCode).toBe(401);
    expect(await findLineConnections(owner.id)).toHaveLength(1);
  });

  it("デモアカウントなら403を返す", async () => {
    getSession.mockResolvedValue(demoSession);

    expect((await request(app, "DELETE", "/api/line/connection")).statusCode).toBe(403);
  });

  it("LINE通知だけを無効にし、連携情報・nonce・OAuth試行を削除する", async () => {
    await createLineConnection(owner.id);
    await createLineLinkNonce(owner.id);
    await createLineOAuthAttempt(owner.id);
    await createNotificationPreference(owner.id, {
      morningEnabled: true,
      lineMorningEnabled: true,
      lineEveningEnabled: true,
    });
    const other = await createUser();
    await createLineConnection(other.id);

    const res = await request(app, "DELETE", "/api/line/connection");

    expect(res.statusCode).toBe(200);
    expect(res.json()).toEqual({ connected: false });
    // メール通知は触らない
    expect(await findNotificationPreferences(owner.id)).toEqual([
      expect.objectContaining({
        morningEnabled: true,
        lineMorningEnabled: false,
        lineEveningEnabled: false,
      }),
    ]);
    expect(await findLineConnections(owner.id)).toHaveLength(0);
    expect(await findLineLinkNonces(owner.id)).toHaveLength(0);
    expect(await findLineOAuthAttempts(owner.id)).toHaveLength(0);
    // 他人の連携は残る
    expect(await findLineConnections(other.id)).toHaveLength(1);
  });
});

describe("GET /api/line/oauth/start", () => {
  let state: string;

  beforeEach(() => {
    state = `state-${crypto.randomUUID()}`;
    vi.mocked(createLineOAuthValues).mockReturnValue({
      state,
      nonce: "nonce-1",
      codeVerifier: "verifier-1",
      codeChallenge: "challenge-1",
    });
    vi.mocked(lineLoginAuthorizationUrl).mockReturnValue(
      new URL(`https://access.line.me/oauth2/v2.1/authorize?state=${state}`)
    );
  });

  it("未ログインなら通知設定へ戻るログイン導線へ送る", async () => {
    getSession.mockResolvedValue(null);

    const res = await request(app, "GET", "/api/line/oauth/start");

    expect(res.headers.location).toBe(
      `${ORIGIN}/login?callbackURL=%2Fprofile%23line-connection`
    );
    expect(await findLineOAuthAttempts(owner.id)).toHaveLength(0);
  });

  it("認証済みユーザーに10分間の単回OAuth試行を作る（古い試行は捨てる）", async () => {
    await createLineOAuthAttempt(owner.id);

    const res = await request(app, "GET", "/api/line/oauth/start");

    expect(res.headers.location).toContain(
      "access.line.me/oauth2/v2.1/authorize"
    );
    const attempts = await findLineOAuthAttempts(owner.id);
    expect(attempts).toEqual([
      expect.objectContaining({
        state,
        nonce: "nonce-1",
        codeVerifier: "verifier-1",
        redirectUri: `${ORIGIN}/api/line/oauth/callback`,
      }),
    ]);
    expect(attempts[0].expiresAt.getTime()).toBeGreaterThan(Date.now() + 9 * 60 * 1000);
  });
});

describe("GET /api/line/oauth/callback", () => {
  let state: string;
  let lineUserId: string;

  const callback = () =>
    request(app, "GET", `/api/line/oauth/callback?code=code&state=${state}`);

  beforeEach(async () => {
    state = await createLineOAuthAttempt(owner.id);
    lineUserId = newLineUserId();
    vi.mocked(exchangeLineLoginCode).mockResolvedValue({
      access_token: "access",
      id_token: "id-token",
    } as never);
    vi.mocked(verifyLineIdToken).mockResolvedValue({
      sub: lineUserId,
      nonce: "nonce-1",
    } as never);
    vi.mocked(getLineFriendshipStatus).mockResolvedValue({ friendFlag: true });
    vi.mocked(pushLineText).mockResolvedValue(undefined);
  });

  it("友だち状態とID tokenを確認して連携し、stateを使い捨てにする", async () => {
    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=connected#line-connection`
    );
    expect(await findLineOAuthAttempts(owner.id)).toHaveLength(0);
    expect(await findLineConnections(owner.id)).toEqual([
      expect.objectContaining({ lineUserId }),
    ]);
    expect(pushLineText).toHaveBeenCalledWith(
      lineUserId,
      `受験マップとのLINE連携が完了しました！\n\n朝・夜の通知は、受験マップのプロフィールから設定できます。\n${ORIGIN}/line/settings`,
      expect.any(AbortSignal)
    );
  });

  it("確認メッセージの送信に失敗しても連携は成功扱いにする", async () => {
    vi.mocked(pushLineText).mockRejectedValue(new Error("LINE API unavailable"));
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=connected#line-connection`
    );
    expect(await findLineConnections(owner.id)).toHaveLength(1);
    expect(consoleError).toHaveBeenCalledWith(
      "[line-oauth] LINE connection completed, but confirmation message failed.",
      expect.any(Error)
    );
    consoleError.mockRestore();
  });

  it("確認メッセージが3秒以内に完了しなくても連携成功画面へ戻す", async () => {
    // 偽の時計にするのは setTimeout / clearTimeout だけ。DB ドライバの動作には触れない。
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    let markPushStarted: () => void = () => {};
    const pushStarted = new Promise<void>((resolve) => {
      markPushStarted = resolve;
    });
    vi.mocked(pushLineText).mockImplementation(
      (_lineUserId, _text, signal) =>
        new Promise((_, reject) => {
          markPushStarted();
          signal?.addEventListener("abort", () =>
            reject(new DOMException("Aborted", "AbortError"))
          );
        })
    );
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    try {
      const responsePromise = callback();
      await pushStarted;
      await vi.advanceTimersByTimeAsync(3_000);
      const res = await responsePromise;

      expect(res.headers.location).toBe(
        `${ORIGIN}/profile?line=connected#line-connection`
      );
      expect(await findLineConnections(owner.id)).toHaveLength(1);
      expect(consoleError).toHaveBeenCalledWith(
        "[line-oauth] LINE connection completed, but confirmation message failed.",
        expect.objectContaining({ name: "AbortError" })
      );
    } finally {
      consoleError.mockRestore();
      vi.useRealTimers();
    }
  });

  it("公式アカウントが友だちでなければ連携しない", async () => {
    vi.mocked(getLineFriendshipStatus).mockResolvedValue({ friendFlag: false });

    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=friend-required#line-connection`
    );
    expect(await findLineConnections(owner.id)).toHaveLength(0);
    expect(pushLineText).not.toHaveBeenCalled();
  });

  it("そのLINEが別ユーザーに連携済みなら連携しない", async () => {
    const other = await createUser();
    await createLineConnection(other.id, lineUserId);

    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=already-used#line-connection`
    );
    expect(await findLineConnections(owner.id)).toHaveLength(0);
    expect(await findLineConnections(other.id)).toEqual([
      expect.objectContaining({ lineUserId }),
    ]);
  });

  it("別ユーザーのstateは利用できず、そのstateは捨てる", async () => {
    const other = await createUser();
    state = await createLineOAuthAttempt(other.id);

    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=expired#line-connection`
    );
    expect(exchangeLineLoginCode).not.toHaveBeenCalled();
    expect(await findLineOAuthAttempts(other.id)).toHaveLength(0);
  });

  it("期限切れのstateは利用できない", async () => {
    state = await createLineOAuthAttempt(owner.id, {
      expiresAt: new Date(Date.now() - 1_000),
    });

    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=expired#line-connection`
    );
    expect(exchangeLineLoginCode).not.toHaveBeenCalled();
  });

  it("セッション切れならcallbackURLを保持してログインへ送り、stateは残す", async () => {
    getSession.mockResolvedValue(null);

    const res = await callback();

    expect(res.headers.location).toContain("/login?callbackURL=");
    expect(await findLineOAuthAttempts(owner.id)).toEqual([expect.objectContaining({ state })]);
  });
});
