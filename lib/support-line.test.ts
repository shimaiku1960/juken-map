import { describe, expect, it } from "vitest";
import {
  createSupportLineCode,
  hasSupportLineAccess,
  hashSupportLineCode,
  supportLineCodeExpiresAt,
} from "@/lib/support-line";

describe("support LINE connection", () => {
  it("無料体験中と契約中だけ利用できる", () => {
    expect(hasSupportLineAccess("trialing")).toBe(true);
    expect(hasSupportLineAccess("active")).toBe(true);
    expect(hasSupportLineAccess("past_due")).toBe(false);
    expect(hasSupportLineAccess("canceled")).toBe(false);
  });

  it("照合コードを正規化してハッシュ化する", () => {
    expect(hashSupportLineCode(" abcd1234 ")).toBe(
      hashSupportLineCode("ABCD1234")
    );
  });

  it("10文字のコードを作成する", () => {
    expect(createSupportLineCode()).toMatch(/^[A-Z0-9_-]{10}$/);
  });

  it("30分後を有効期限にする", () => {
    const now = new Date("2026-08-11T08:30:00.000Z");
    expect(supportLineCodeExpiresAt(now).toISOString()).toBe(
      "2026-08-11T09:00:00.000Z"
    );
  });
});
