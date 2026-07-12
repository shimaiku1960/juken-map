import { describe, expect, it } from "vitest";
import { goalSchema, patchGoalSchema } from "@/lib/validations/goal";

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

  it("status は candidate / decided を通す", () => {
    expect(goalSchema.safeParse({ facultyId: 1, status: "candidate" }).success).toBe(true);
    expect(goalSchema.safeParse({ facultyId: 1, status: "decided" }).success).toBe(true);
  });

  it("status は省略可能", () => {
    expect(goalSchema.safeParse({ facultyId: 1 }).success).toBe(true);
  });

  it("未知の status は弾く", () => {
    expect(goalSchema.safeParse({ facultyId: 1, status: "maybe" }).success).toBe(false);
  });
});

describe("patchGoalSchema", () => {
  it("status だけの部分更新を通す", () => {
    expect(patchGoalSchema.safeParse({ status: "decided" }).success).toBe(true);
  });

  it("未知の status は弾く", () => {
    expect(patchGoalSchema.safeParse({ status: "foo" }).success).toBe(false);
  });
});