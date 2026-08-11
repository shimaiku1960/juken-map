import { createHash, randomBytes } from "node:crypto";

export const SUPPORT_LINE_CODE_TTL_MINUTES = 30;

export function hasSupportLineAccess(status: string) {
  return status === "trialing" || status === "active";
}

export function createSupportLineCode() {
  return randomBytes(8).toString("base64url").toUpperCase().slice(0, 10);
}

export function hashSupportLineCode(code: string) {
  return createHash("sha256")
    .update(code.trim().toUpperCase())
    .digest("hex");
}

export function supportLineCodeExpiresAt(now = new Date()) {
  return new Date(now.getTime() + SUPPORT_LINE_CODE_TTL_MINUTES * 60 * 1000);
}
