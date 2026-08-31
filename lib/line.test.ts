import { createHmac } from "node:crypto";
import { beforeEach, describe, expect, it } from "vitest";
import { verifyLineSignature } from "@/lib/line";

beforeEach(() => {
  process.env.LINE_CHANNEL_SECRET = "test-channel-secret";
});

describe("verifyLineSignature", () => {
  it("本文のHMAC-SHA256署名が一致する場合だけ受理する", () => {
    const body = JSON.stringify({ events: [] });
    const signature = createHmac("sha256", "test-channel-secret").update(body).digest("base64");
    expect(verifyLineSignature(body, signature)).toBe(true);
    expect(verifyLineSignature(`${body} `, signature)).toBe(false);
    expect(verifyLineSignature(body, null)).toBe(false);
  });
});
