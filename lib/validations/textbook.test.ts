import { describe, it, expect } from "vitest";
import { createTextbookSchema } from "@/lib/validations/textbook";

describe("createTextbookSchema", () => {
  it("通常の参考書名を通す", () => {
    const result = createTextbookSchema.safeParse({ name: "青チャートIA" });
    expect(result.success).toBe(true);
  });

  it("name が空文字なら弾く", () => {
    const result = createTextbookSchema.safeParse({ name: "" });
    expect(result.success).toBe(false);
  });

  it("name が無ければ弾く", () => {
    const result = createTextbookSchema.safeParse({});
    expect(result.success).toBe(false);
  });

  it("前後の空白は trim される", () => {
    const result = createTextbookSchema.safeParse({ name: "  数学  " });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.name).toBe("数学");
    }
  });

  it("100文字ちょうどは通す（境界）", () => {
    const result = createTextbookSchema.safeParse({ name: "a".repeat(100) });
    expect(result.success).toBe(true);
  });

  it("101文字なら弾く（境界）", () => {
    const result = createTextbookSchema.safeParse({ name: "a".repeat(101) });
    expect(result.success).toBe(false);
  });

  // trim を min の前に置いているので、空白のみは trim 後に空文字となり弾かれる
  it("空白のみは弾く", () => {
    const result = createTextbookSchema.safeParse({ name: "   " });
    expect(result.success).toBe(false);
  });
});
