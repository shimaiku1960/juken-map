import { createHash, randomBytes } from "node:crypto";

const LINE_LOGIN_API = "https://api.line.me";
const LINE_LOGIN_AUTHORIZE = "https://access.line.me/oauth2/v2.1/authorize";

function requiredEnv(name: "LINE_LOGIN_CHANNEL_ID" | "LINE_LOGIN_CHANNEL_SECRET") {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is not configured`);
  return value;
}

export function createLineOAuthValues() {
  const state = randomBytes(32).toString("base64url");
  const nonce = randomBytes(32).toString("base64url");
  const codeVerifier = randomBytes(64).toString("base64url");
  const codeChallenge = createHash("sha256").update(codeVerifier).digest("base64url");
  return { state, nonce, codeVerifier, codeChallenge };
}

export function lineLoginAuthorizationUrl(input: {
  state: string;
  nonce: string;
  codeChallenge: string;
  redirectUri: string;
}) {
  const url = new URL(LINE_LOGIN_AUTHORIZE);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("client_id", requiredEnv("LINE_LOGIN_CHANNEL_ID"));
  url.searchParams.set("redirect_uri", input.redirectUri);
  url.searchParams.set("state", input.state);
  url.searchParams.set("scope", "openid profile");
  url.searchParams.set("nonce", input.nonce);
  url.searchParams.set("bot_prompt", "aggressive");
  url.searchParams.set("code_challenge", input.codeChallenge);
  url.searchParams.set("code_challenge_method", "S256");
  return url;
}

type LineTokenResponse = { access_token: string; id_token: string };
type VerifiedLineIdToken = { sub: string; nonce?: string };

async function lineJson<T>(url: string, init: RequestInit): Promise<T> {
  const response = await fetch(url, { ...init, cache: "no-store" });
  if (!response.ok) {
    const detail = await response.text();
    throw new Error(`LINE Login API ${response.status}: ${detail.slice(0, 300)}`);
  }
  return response.json() as Promise<T>;
}

export async function exchangeLineLoginCode(input: {
  code: string;
  codeVerifier: string;
  redirectUri: string;
}) {
  const channelId = requiredEnv("LINE_LOGIN_CHANNEL_ID");
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code: input.code,
    redirect_uri: input.redirectUri,
    client_id: channelId,
    client_secret: requiredEnv("LINE_LOGIN_CHANNEL_SECRET"),
    code_verifier: input.codeVerifier,
  });
  return lineJson<LineTokenResponse>(`${LINE_LOGIN_API}/oauth2/v2.1/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
}

export async function verifyLineIdToken(idToken: string, nonce: string) {
  const body = new URLSearchParams({
    id_token: idToken,
    client_id: requiredEnv("LINE_LOGIN_CHANNEL_ID"),
    nonce,
  });
  return lineJson<VerifiedLineIdToken>(`${LINE_LOGIN_API}/oauth2/v2.1/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
}

export async function getLineFriendshipStatus(accessToken: string) {
  return lineJson<{ friendFlag: boolean }>(`${LINE_LOGIN_API}/friendship/v1/status`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
}
