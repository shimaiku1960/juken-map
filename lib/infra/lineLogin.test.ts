import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createLineOAuthValues,
  exchangeLineLoginCode,
  getLineFriendshipStatus,
  lineLoginAuthorizationUrl,
  verifyLineIdToken,
} from "./lineLogin";

beforeEach(() => {
  process.env.LINE_LOGIN_CHANNEL_ID = "line-login-channel";
  process.env.LINE_LOGIN_CHANNEL_SECRET = "line-login-secret";
});

afterEach(() => vi.unstubAllGlobals());

describe("LINE Login", () => {
  it("PKCEと再利用しにくいstate・nonceを生成する", () => {
    const first = createLineOAuthValues();
    const second = createLineOAuthValues();

    expect(first.state).not.toBe(second.state);
    expect(first.nonce).not.toBe(second.nonce);
    expect(first.codeVerifier.length).toBeGreaterThanOrEqual(43);
    expect(first.codeChallenge).toMatch(/^[A-Za-z0-9_-]+$/);
  });

  it("最小scope、友だち追加、PKCEを含む認可URLを作る", () => {
    const url = lineLoginAuthorizationUrl({
      state: "state-1",
      nonce: "nonce-1",
      codeChallenge: "challenge-1",
      redirectUri: "https://juken-map.com/api/line/oauth/callback",
    });

    expect(url.origin + url.pathname).toBe("https://access.line.me/oauth2/v2.1/authorize");
    expect(url.searchParams.get("client_id")).toBe("line-login-channel");
    expect(url.searchParams.get("scope")).toBe("openid profile");
    expect(url.searchParams.get("bot_prompt")).toBe("aggressive");
    expect(url.searchParams.get("code_challenge_method")).toBe("S256");
  });

  it("認可コード交換、ID token検証、友だち状態確認をサーバーで行う", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ access_token: "access", id_token: "id-token" })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ sub: "U123", nonce: "nonce-1" })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ friendFlag: true })));
    vi.stubGlobal("fetch", fetchMock);

    const tokens = await exchangeLineLoginCode({ code: "code", codeVerifier: "verifier", redirectUri: "https://juken-map.com/api/line/oauth/callback" });
    const identity = await verifyLineIdToken(tokens.id_token, "nonce-1");
    const friendship = await getLineFriendshipStatus(tokens.access_token);

    expect(identity.sub).toBe("U123");
    expect(friendship.friendFlag).toBe(true);
    const tokenBody = fetchMock.mock.calls[0][1].body as URLSearchParams;
    expect(tokenBody.get("client_secret")).toBe("line-login-secret");
    expect(tokenBody.get("code_verifier")).toBe("verifier");
    expect(fetchMock.mock.calls[2][1].headers).toEqual({ Authorization: "Bearer access" });
  });

  it("LINE APIエラーを成功扱いにしない", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("invalid_grant", { status: 400 })));
    await expect(exchangeLineLoginCode({ code: "bad", codeVerifier: "verifier", redirectUri: "https://juken-map.com/api/line/oauth/callback" }))
      .rejects.toThrow("LINE Login API 400");
  });
});
