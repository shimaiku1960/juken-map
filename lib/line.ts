import { createHmac, timingSafeEqual } from "node:crypto";
import { SITE_URL } from "@/lib/site";

const LINE_API_BASE = "https://api.line.me/v2/bot";

function requiredEnv(name: "LINE_CHANNEL_SECRET" | "LINE_CHANNEL_ACCESS_TOKEN") {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is not configured`);
  return value;
}

export function verifyLineSignature(body: string, signature: string | null) {
  if (!signature) return false;
  const expected = createHmac("sha256", requiredEnv("LINE_CHANNEL_SECRET"))
    .update(body)
    .digest("base64");
  const actualBuffer = Buffer.from(signature);
  const expectedBuffer = Buffer.from(expected);
  return (
    actualBuffer.length === expectedBuffer.length &&
    timingSafeEqual(actualBuffer, expectedBuffer)
  );
}

async function lineRequest(path: string, init: RequestInit) {
  const response = await fetch(`${LINE_API_BASE}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${requiredEnv("LINE_CHANNEL_ACCESS_TOKEN")}`,
      "Content-Type": "application/json",
      ...init.headers,
    },
  });
  if (!response.ok) {
    const detail = await response.text();
    throw new Error(`LINE API ${response.status}: ${detail.slice(0, 300)}`);
  }
  return response;
}

export async function issueLineLinkToken(lineUserId: string) {
  const response = await lineRequest(`/user/${encodeURIComponent(lineUserId)}/linkToken`, {
    method: "POST",
  });
  const result = (await response.json()) as { linkToken: string };
  return result.linkToken;
}

export async function replyLineText(replyToken: string, text: string) {
  await lineRequest("/message/reply", {
    method: "POST",
    body: JSON.stringify({
      replyToken,
      messages: [{ type: "text", text }],
    }),
  });
}

export async function pushLineText(lineUserId: string, text: string, signal?: AbortSignal) {
  await lineRequest("/message/push", {
    method: "POST",
    signal,
    body: JSON.stringify({
      to: lineUserId,
      messages: [{ type: "text", text }],
    }),
  });
}

export function lineAccountLinkUrl(linkToken: string) {
  const url = new URL("/line/link", SITE_URL);
  url.searchParams.set("linkToken", linkToken);
  return url.toString();
}
