import { describe, it, expect } from "vitest";
import {
  createTextbookSchema,
  updateTextbookProgressSchema,
} from "@/lib/validations/textbook";

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
      expect("name" in result.data && result.data.name).toBe("数学");
    }
  });

  it("参考書マスターIDを通す", () => {
    expect(createTextbookSchema.safeParse({ masterId: 1 }).success).toBe(true);
  });

  it("不正な参考書マスターIDを弾く", () => {
    expect(createTextbookSchema.safeParse({ masterId: 0 }).success).toBe(false);
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

describe("updateTextbookProgressSchema", () => {
  it("総量と対応単位を通す", () => {
    expect(
      updateTextbookProgressSchema.safeParse({
        totalAmount: 300,
        rangeUnit: "page",
        targetDate: "2026-12-31",
      }).success
    ).toBe(true);
  });

  it("総量が0なら弾く", () => {
    expect(
      updateTextbookProgressSchema.safeParse({
        totalAmount: 0,
        rangeUnit: "page",
        targetDate: null,
      }).success
    ).toBe(false);
  });

  it("未対応の単位なら弾く", () => {
    expect(
      updateTextbookProgressSchema.safeParse({
        totalAmount: 100,
        rangeUnit: "week", // page/question/chapter/number/part/section 以外
        targetDate: null,
      }).success
    ).toBe(false);
  });

  it("完了目標日が未設定でも通す", () => {
    expect(
      updateTextbookProgressSchema.safeParse({
        totalAmount: 100,
        rangeUnit: "page",
        targetDate: null,
      }).success
    ).toBe(true);
  });

  it("不正な完了目標日なら弾く", () => {
    expect(
      updateTextbookProgressSchema.safeParse({
        totalAmount: 100,
        rangeUnit: "page",
        targetDate: "2026-99-99",
      }).success
    ).toBe(false);
  });

  it("科目を指定できる／null・未指定も許容", () => {
    const base = { totalAmount: 100, rangeUnit: "page", targetDate: null };
    expect(
      updateTextbookProgressSchema.safeParse({ ...base, subject: "english" })
        .success
    ).toBe(true);
    expect(
      updateTextbookProgressSchema.safeParse({ ...base, subject: null }).success
    ).toBe(true);
    expect(updateTextbookProgressSchema.safeParse(base).success).toBe(true);
  });

  it("未知の科目なら弾く", () => {
    expect(
      updateTextbookProgressSchema.safeParse({
        totalAmount: 100,
        rangeUnit: "page",
        targetDate: null,
        subject: "history",
      }).success
    ).toBe(false);
  });
});
