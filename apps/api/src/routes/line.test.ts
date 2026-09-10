import { describe, it, expect, beforeEach, vi, type Mock } from "vitest";

vi.mock("../auth.ts", () => ({
  auth: { api: { getSession: vi.fn() } },
}));

const transactionPrisma = {
  lineConnection: { findUnique: vi.fn(), upsert: vi.fn() },
  lineLinkNonce: { delete: vi.fn() },
};

vi.mock("@/api/infra/prisma", () => {
  // routes は名前付き、services は default で import している。同じ実体を返す。
  const client = {
    $transaction: vi.fn(),
    lineConnection: { findUnique: vi.fn(), deleteMany: vi.fn(), upsert: vi.fn() },
    lineLinkNonce: {
      findUnique: vi.fn(),
      create: vi.fn(),
      delete: vi.fn(),
      deleteMany: vi.fn(),
    },
    lineOAuthAttempt: {
      findUnique: vi.fn(),
      create: vi.fn(),
      delete: vi.fn(),
      deleteMany: vi.fn(),
    },
    notificationPreference: { updateMany: vi.fn() },
  };
  return { prisma: client, default: client };
});

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

vi.mock("@/api/services/notification-service", () => ({
  findLineConnection: vi.fn(),
}));

const { auth } = await import("../auth.ts");
const { prisma } = await import("@/api/infra/prisma");
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
const { buildTestApp, request, loggedInSession } = await import(
  "../test-support.ts"
);

const getSession = auth.api.getSession as unknown as Mock;

const app = buildTestApp(registerLineRoutes);

// WEB_ORIGIN 未設定なので webOrigin() は SITE_URL を返す。
const ORIGIN = "https://juken-map.com";

const webhook = (events: unknown[]) =>
  request(app, "POST", "/api/line/webhook", { events }, {
    "x-line-signature": "signature",
  });

beforeEach(() => {
  vi.clearAllMocks();
});

describe("GET /line/settings", () => {
  it("ログイン済みならプロフィールの通知設定へ移動する", async () => {
    getSession.mockResolvedValue(loggedInSession);

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
  it("署名が不正なら401を返す", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(false);

    expect((await webhook([])).statusCode).toBe(401);
  });

  it("連携メッセージへ公式Account Linking URLを返す", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(prisma.lineConnection.findUnique).mockResolvedValue(null);
    vi.mocked(issueLineLinkToken).mockResolvedValue("token");
    vi.mocked(replyLineText).mockResolvedValue(undefined);

    const res = await webhook([
      {
        type: "message",
        replyToken: "reply-token",
        source: { type: "user", userId: "U123" },
        message: { type: "text", text: "連携" },
      },
    ]);

    expect(res.statusCode).toBe(200);
    expect(issueLineLinkToken).toHaveBeenCalledWith("U123");
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("10分以内")
    );
  });

  it("連携済みなら再連携リンクを発行せず通知設定を案内する", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(prisma.lineConnection.findUnique).mockResolvedValue({
      id: 1,
    } as never);
    vi.mocked(replyLineText).mockResolvedValue(undefined);

    const res = await webhook([
      {
        type: "message",
        replyToken: "reply-token",
        source: { type: "user", userId: "U123" },
        message: { type: "text", text: "連携" },
      },
    ]);

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

  it("有効なnonceでアプリユーザーとLINEユーザーを連携する", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(replyLineText).mockResolvedValue(undefined);
    vi.mocked(prisma.lineLinkNonce.findUnique).mockResolvedValue({
      nonce: "nonce-1",
      userId: "user-1",
      expiresAt: new Date(Date.now() + 60_000),
    } as never);
    vi.mocked(prisma.$transaction).mockImplementation(
      (callback: unknown) =>
        (callback as (tx: unknown) => unknown)(transactionPrisma) as never
    );
    transactionPrisma.lineConnection.findUnique.mockResolvedValue(null);
    transactionPrisma.lineConnection.upsert.mockResolvedValue({});
    transactionPrisma.lineLinkNonce.delete.mockResolvedValue({});

    const res = await webhook([
      {
        type: "accountLink",
        replyToken: "reply-token",
        source: { type: "user", userId: "U123" },
        link: { result: "ok", nonce: "nonce-1" },
      },
    ]);

    expect(res.statusCode).toBe(200);
    expect(transactionPrisma.lineConnection.upsert).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      create: { userId: "user-1", lineUserId: "U123" },
      update: { lineUserId: "U123", linkedAt: expect.any(Date) },
    });
    expect(transactionPrisma.lineLinkNonce.delete).toHaveBeenCalledWith({
      where: { nonce: "nonce-1" },
    });
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("連携が完了")
    );
  });

  it("別ユーザーに連携済みなら解除方法を返信する", async () => {
    vi.mocked(verifyLineSignature).mockReturnValue(true);
    vi.mocked(replyLineText).mockResolvedValue(undefined);
    vi.mocked(prisma.lineLinkNonce.findUnique).mockResolvedValue({
      nonce: "nonce-2",
      userId: "user-2",
      expiresAt: new Date(Date.now() + 60_000),
    } as never);
    vi.mocked(prisma.$transaction).mockImplementation(
      (callback: unknown) =>
        (callback as (tx: unknown) => unknown)(transactionPrisma) as never
    );
    transactionPrisma.lineConnection.findUnique.mockResolvedValue({
      userId: "user-1",
    });
    transactionPrisma.lineLinkNonce.delete.mockResolvedValue({});

    const res = await webhook([
      {
        type: "accountLink",
        replyToken: "reply-token",
        source: { type: "user", userId: "U123" },
        link: { result: "ok", nonce: "nonce-2" },
      },
    ]);

    expect(res.statusCode).toBe(200);
    expect(transactionPrisma.lineConnection.upsert).not.toHaveBeenCalled();
    expect(replyLineText).toHaveBeenCalledWith(
      "reply-token",
      expect.stringContaining("別の受験マップアカウント")
    );
  });
});

describe("POST /api/line/account-link", () => {
  it("未ログインならnonceを発行しない", async () => {
    getSession.mockResolvedValue(null);

    const res = await request(app, "POST", "/api/line/account-link", {
      linkToken: "token",
    });

    expect(res.statusCode).toBe(401);
    expect(prisma.lineLinkNonce.create).not.toHaveBeenCalled();
  });

  it("ログインユーザーに10分間の単回nonceを発行する", async () => {
    getSession.mockResolvedValue(loggedInSession);
    vi.mocked(prisma.$transaction).mockResolvedValue([] as never);
    const before = Date.now();

    const res = await request(app, "POST", "/api/line/account-link", {
      linkToken: "link-token",
    });

    expect(res.statusCode).toBe(200);
    expect(res.json().redirectUrl).toContain(
      "https://access.line.me/dialog/bot/accountLink"
    );
    expect(prisma.lineLinkNonce.create).toHaveBeenCalledWith({
      data: expect.objectContaining({
        userId: "user-1",
        nonce: expect.any(String),
        expiresAt: expect.any(Date),
      }),
    });
    expect(prisma.lineLinkNonce.deleteMany).toHaveBeenCalledWith({
      where: { userId: "user-1" },
    });
    const { expiresAt } = vi.mocked(prisma.lineLinkNonce.create).mock
      .calls[0][0].data as { expiresAt: Date };
    expect(expiresAt.getTime()).toBeGreaterThanOrEqual(before + 9 * 60 * 1000);
  });
});

describe("DELETE /api/line/connection", () => {
  it("未ログインなら連携を解除しない", async () => {
    getSession.mockResolvedValue(null);

    const res = await request(app, "DELETE", "/api/line/connection");

    expect(res.statusCode).toBe(401);
    expect(prisma.$transaction).not.toHaveBeenCalled();
  });

  it("LINE設定だけを無効にして連携情報を削除する", async () => {
    getSession.mockResolvedValue(loggedInSession);
    vi.mocked(prisma.$transaction).mockResolvedValue([] as never);

    const res = await request(app, "DELETE", "/api/line/connection");

    expect(res.statusCode).toBe(200);
    expect(prisma.notificationPreference.updateMany).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      data: { lineMorningEnabled: false, lineEveningEnabled: false },
    });
    expect(prisma.$transaction).toHaveBeenCalledOnce();
  });
});

describe("GET /api/line/oauth/start", () => {
  beforeEach(() => {
    vi.mocked(createLineOAuthValues).mockReturnValue({
      state: "state-1",
      nonce: "nonce-1",
      codeVerifier: "verifier-1",
      codeChallenge: "challenge-1",
    });
    vi.mocked(lineLoginAuthorizationUrl).mockReturnValue(
      new URL("https://access.line.me/oauth2/v2.1/authorize?state=state-1")
    );
    vi.mocked(prisma.$transaction).mockResolvedValue([] as never);
  });

  it("未ログインなら通知設定へ戻るログイン導線へ送る", async () => {
    getSession.mockResolvedValue(null);

    const res = await request(app, "GET", "/api/line/oauth/start");

    expect(res.headers.location).toBe(
      `${ORIGIN}/login?callbackURL=%2Fprofile%23line-connection`
    );
    expect(prisma.$transaction).not.toHaveBeenCalled();
  });

  it("認証済みユーザーに10分間の単回OAuth試行を作る", async () => {
    getSession.mockResolvedValue(loggedInSession);

    const res = await request(app, "GET", "/api/line/oauth/start");

    expect(res.headers.location).toContain(
      "access.line.me/oauth2/v2.1/authorize"
    );
    expect(prisma.lineOAuthAttempt.deleteMany).toHaveBeenCalledWith({
      where: { userId: "user-1" },
    });
    expect(prisma.lineOAuthAttempt.create).toHaveBeenCalledWith({
      data: expect.objectContaining({
        state: "state-1",
        nonce: "nonce-1",
        codeVerifier: "verifier-1",
        userId: "user-1",
        expiresAt: expect.any(Date),
      }),
    });
  });
});

describe("GET /api/line/oauth/callback", () => {
  const attempt = {
    state: "state-1",
    userId: "user-1",
    nonce: "nonce-1",
    codeVerifier: "verifier",
    redirectUri: `${ORIGIN}/api/line/oauth/callback`,
    expiresAt: new Date(Date.now() + 60_000),
  };

  const callback = () =>
    request(app, "GET", "/api/line/oauth/callback?code=code&state=state-1");

  const connectedTx = () => {
    const tx = {
      lineConnection: {
        findUnique: vi.fn().mockResolvedValue(null),
        upsert: vi.fn().mockResolvedValue({}),
      },
    };
    vi.mocked(prisma.$transaction).mockImplementation(
      (callback: unknown) =>
        (callback as (tx: unknown) => unknown)(tx) as never
    );
    return tx;
  };

  beforeEach(() => {
    getSession.mockResolvedValue(loggedInSession);
    vi.mocked(prisma.lineOAuthAttempt.findUnique).mockResolvedValue(
      attempt as never
    );
    vi.mocked(prisma.lineOAuthAttempt.delete).mockResolvedValue(
      attempt as never
    );
    vi.mocked(exchangeLineLoginCode).mockResolvedValue({
      access_token: "access",
      id_token: "id-token",
    } as never);
    vi.mocked(verifyLineIdToken).mockResolvedValue({
      sub: "U123",
      nonce: "nonce-1",
    } as never);
    vi.mocked(getLineFriendshipStatus).mockResolvedValue({ friendFlag: true });
    vi.mocked(pushLineText).mockResolvedValue(undefined);
  });

  it("友だち状態とID tokenを確認して連携する", async () => {
    const tx = connectedTx();

    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=connected#line-connection`
    );
    // state は使い捨て。取得と削除の間に別リクエストが消していても落ちないよう
    // deleteMany を使う（delete は該当行が無いと例外になる）。
    expect(prisma.lineOAuthAttempt.deleteMany).toHaveBeenCalledWith({
      where: { state: "state-1" },
    });
    expect(tx.lineConnection.upsert).toHaveBeenCalledWith({
      where: { userId: "user-1" },
      create: { userId: "user-1", lineUserId: "U123" },
      update: { lineUserId: "U123", linkedAt: expect.any(Date) },
    });
    expect(pushLineText).toHaveBeenCalledWith(
      "U123",
      `受験マップとのLINE連携が完了しました！\n\n朝・夜の通知は、受験マップのプロフィールから設定できます。\n${ORIGIN}/line/settings`,
      expect.any(AbortSignal)
    );
  });

  it("確認メッセージの送信に失敗しても連携は成功扱いにする", async () => {
    const tx = connectedTx();
    vi.mocked(pushLineText).mockRejectedValue(new Error("LINE API unavailable"));
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=connected#line-connection`
    );
    expect(tx.lineConnection.upsert).toHaveBeenCalled();
    expect(consoleError).toHaveBeenCalledWith(
      "[line-oauth] LINE connection completed, but confirmation message failed.",
      expect.any(Error)
    );
    consoleError.mockRestore();
  });

  it("確認メッセージが3秒以内に完了しなくても連携成功画面へ戻す", async () => {
    vi.useFakeTimers();
    const tx = connectedTx();
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
      expect(tx.lineConnection.upsert).toHaveBeenCalled();
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
    expect(prisma.$transaction).not.toHaveBeenCalled();
    expect(pushLineText).not.toHaveBeenCalled();
  });

  it("別ユーザーのstateは利用できない", async () => {
    vi.mocked(prisma.lineOAuthAttempt.findUnique).mockResolvedValue({
      ...attempt,
      userId: "other-user",
    } as never);

    const res = await callback();

    expect(res.headers.location).toBe(
      `${ORIGIN}/profile?line=expired#line-connection`
    );
    expect(exchangeLineLoginCode).not.toHaveBeenCalled();
  });

  it("セッション切れならcallbackURLを保持してログインへ送る", async () => {
    getSession.mockResolvedValue(null);

    const res = await callback();

    expect(res.headers.location).toContain("/login?callbackURL=");
    expect(prisma.lineOAuthAttempt.findUnique).not.toHaveBeenCalled();
  });
});
