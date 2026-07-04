import { describe, expect, it } from "vitest";
import { goalSchema } from "@/lib/validations/goal";

describe("goalSchema", () => {
  it("正しい facultyId を通す", () => {
    const result = goalSchema.safeParse({ facultyId: 1 });
    expect(result.success).toBe(true);
  });

  it("facultyId が 0 以下なら弾く", () => {
    const result = goalSchema.safeParse({ facultyId: 0 });
    expect(result.success).toBe(false);
  });

  it("facultyId が小数なら弾く", () => {
    const result = goalSchema.safeParse({ facultyId: 1.5 });
    expect(result.success).toBe(false);
  });

  it("facultyId が無ければ弾く", () => {
    const result = goalSchema.safeParse({});
    expect(result.success).toBe(false);
  });
});