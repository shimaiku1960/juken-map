import { describe, it, expect } from "vitest";
import { profileSchema } from "@/lib/validations/profile";

describe("profileSchema", () => {
  it("正常なニックネームを通す", () => {
    const result = profileSchema.safeParse({ nickname: "デモ太郎" });
    expect(result.success).toBe(true);
  });

  it("nickname が空なら弾く", () => {
    const result = profileSchema.safeParse({ nickname: "" });
    expect(result.success).toBe(false);
  });

  it("nickname が無ければ弾く", () => {
    const result = profileSchema.safeParse({});
    expect(result.success).toBe(false);
  });

  it("nickname が文字列でなければ弾く", () => {
    const result = profileSchema.safeParse({ nickname: 123 });
    expect(result.success).toBe(false);
  });

  it("50文字ちょうどは通す（境界）", () => {
    const result = profileSchema.safeParse({ nickname: "あ".repeat(50) });
    expect(result.success).toBe(true);
  });

  it("51文字なら弾く（境界）", () => {
    const result = profileSchema.safeParse({ nickname: "あ".repeat(51) });
    expect(result.success).toBe(false);
  });

  it("前後の空白は trim される", () => {
    const result = profileSchema.safeParse({ nickname: "  デモ太郎  " });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.nickname).toBe("デモ太郎");
    }
  });
});
