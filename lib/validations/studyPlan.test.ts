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

describe("studyPlanItem 範囲・参考書バリデーション", () => {
  // date + items でラップして 1 件分の item を検証するヘルパー
  const parseItem = (item: unknown) =>
    createStudyPlansSchema.safeParse({ date: "2027-02-20", items: [item] });

  it("参考書＋範囲＋単位が揃っていれば通す", () => {
    const result = parseItem({
      textbookId: 1,
      rangeStart: 10,
      rangeEnd: 20,
      rangeUnit: "page",
    });
    expect(result.success).toBe(true);
  });

  it("参考書だけ（範囲なし）でも通す", () => {
    const result = parseItem({ textbookId: 1 });
    expect(result.success).toBe(true);
  });

  it("中身が全部空（参考書・範囲・メモなし）なら弾く", () => {
    const result = parseItem({});
    expect(result.success).toBe(false);
  });

  it("開始だけで終了が無ければ弾く", () => {
    const result = parseItem({ rangeStart: 10, rangeUnit: "page" });
    expect(result.success).toBe(false);
  });

  it("終了だけで開始が無ければ弾く", () => {
    const result = parseItem({ rangeEnd: 20, rangeUnit: "page" });
    expect(result.success).toBe(false);
  });

  it("開始 > 終了なら弾く", () => {
    const result = parseItem({
      rangeStart: 30,
      rangeEnd: 20,
      rangeUnit: "page",
    });
    expect(result.success).toBe(false);
  });

  it("開始 = 終了は通す（境界・1ページだけ）", () => {
    const result = parseItem({
      rangeStart: 15,
      rangeEnd: 15,
      rangeUnit: "page",
    });
    expect(result.success).toBe(true);
  });

  it("範囲はあるのに単位が無ければ弾く", () => {
    const result = parseItem({ rangeStart: 10, rangeEnd: 20 });
    expect(result.success).toBe(false);
  });

  it("単位が不正な値なら弾く", () => {
    const result = parseItem({
      rangeStart: 10,
      rangeEnd: 20,
      rangeUnit: "not-a-unit",
    });
    expect(result.success).toBe(false);
  });

  it("rangeStart が 0 以下なら弾く", () => {
    const result = parseItem({
      rangeStart: 0,
      rangeEnd: 20,
      rangeUnit: "page",
    });
    expect(result.success).toBe(false);
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

  it("content が空文字でも通す（メモは任意）", () => {
    const result = updateStudyPlanSchema.safeParse({ content: "" });
    expect(result.success).toBe(true);
  });

  it("subject が不正な値なら弾く", () => {
    const result = updateStudyPlanSchema.safeParse({ subject: "invalid" });
    expect(result.success).toBe(false);
  });

  it("範囲の更新（開始・終了・単位）を通す", () => {
    const result = updateStudyPlanSchema.safeParse({
      rangeStart: 5,
      rangeEnd: 12,
      rangeUnit: "question",
    });
    expect(result.success).toBe(true);
  });

  it("開始 > 終了の更新は弾く", () => {
    const result = updateStudyPlanSchema.safeParse({
      rangeStart: 12,
      rangeEnd: 5,
      rangeUnit: "question",
    });
    expect(result.success).toBe(false);
  });
});
