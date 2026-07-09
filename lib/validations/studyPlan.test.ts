import { describe, it, expect } from "vitest";
import {
  createStudyPlansSchema,
  updateStudyPlanSchema,
} from "@/lib/validations/studyPlan";

describe("createStudyPlansSchema", () => {
  it("正常な入力を通す", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [{ content: "英単語", subject: "english" }],
    });
    expect(result.success).toBe(true);
  });

  it("date が無ければ弾く", () => {
    const result = createStudyPlansSchema.safeParse({
      items: [{ content: "英単語" }],
    });
    expect(result.success).toBe(false);
  });

  it("items が空配列なら弾く", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [],
    });
    expect(result.success).toBe(false);
  });

  it("content が空なら弾く", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [{ content: "" }],
    });
    expect(result.success).toBe(false);
  });

  it("content が500文字ちょうどは通す（境界）", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [{ content: "a".repeat(500) }],
    });
    expect(result.success).toBe(true);
  });

  it("content が501文字なら弾く（境界）", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [{ content: "a".repeat(501) }],
    });
    expect(result.success).toBe(false);
  });

  it("content の前後の空白は trim される", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [{ content: "  英単語  " }],
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.items[0].content).toBe("英単語");
    }
  });

  it("subject が不正な値なら弾く", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [{ content: "英単語", subject: "not-a-subject" }],
    });
    expect(result.success).toBe(false);
  });

  it("subject が null は通す（未設定）", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [{ content: "英単語", subject: null }],
    });
    expect(result.success).toBe(true);
  });

  it("subject を省略しても通す", () => {
    const result = createStudyPlansSchema.safeParse({
      date: "2027-02-20",
      items: [{ content: "英単語" }],
    });
    expect(result.success).toBe(true);
  });
});

describe("updateStudyPlanSchema", () => {
  it("done だけの部分更新を通す", () => {
    const result = updateStudyPlanSchema.safeParse({ done: true });
    expect(result.success).toBe(true);
  });

  it("空オブジェクト（何も更新しない）も通す", () => {
    const result = updateStudyPlanSchema.safeParse({});
    expect(result.success).toBe(true);
  });

  it("done が boolean 以外なら弾く", () => {
    const result = updateStudyPlanSchema.safeParse({ done: "yes" });
    expect(result.success).toBe(false);
  });

  it("content が空文字なら弾く", () => {
    const result = updateStudyPlanSchema.safeParse({ content: "" });
    expect(result.success).toBe(false);
  });

  it("subject が不正な値なら弾く", () => {
    const result = updateStudyPlanSchema.safeParse({ subject: "invalid" });
    expect(result.success).toBe(false);
  });
});
