import { describe, it, expect } from "vitest";
import {
  completeStudyPlanSchema,
  createStudyLogSchema,
} from "@/lib/validations/studyLog";

describe("createStudyLogSchema", () => {
  it("正常な入力を通す（時間のみ）", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 60,
      subject: "english",
    });
    expect(result.success).toBe(true);
  });

  it("正常な入力を通す（参考書＋範囲＋メモ付き）", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 90,
      subject: "math",
      textbookId: 3,
      rangeStart: 10,
      rangeEnd: 20,
      rangeUnit: "page",
      memo: "青チャート",
    });
    expect(result.success).toBe(true);
  });

  it("date が無ければ弾く", () => {
    const result = createStudyLogSchema.safeParse({ minutes: 60 });
    expect(result.success).toBe(false);
  });

  it("minutes が無ければ弾く", () => {
    const result = createStudyLogSchema.safeParse({ date: "2027-02-20" });
    expect(result.success).toBe(false);
  });

  it("minutes が 0 なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 0,
    });
    expect(result.success).toBe(false);
  });

  it("minutes が負なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: -30,
    });
    expect(result.success).toBe(false);
  });

  it("minutes が 1440 ちょうどは通す（境界）", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 1440,
    });
    expect(result.success).toBe(true);
  });

  it("minutes が 1441 なら弾く（境界超え）", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 1441,
    });
    expect(result.success).toBe(false);
  });

  it("minutes が小数なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 30.5,
    });
    expect(result.success).toBe(false);
  });

  it("不正な科目は弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 60,
      subject: "cooking",
    });
    expect(result.success).toBe(false);
  });

  it("範囲が片側だけなら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 60,
      rangeStart: 10,
      rangeUnit: "page",
    });
    expect(result.success).toBe(false);
  });

  it("範囲を入れたのに単位が無ければ弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 60,
      rangeStart: 10,
      rangeEnd: 20,
    });
    expect(result.success).toBe(false);
  });

  it("開始 > 終了 なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 60,
      rangeStart: 30,
      rangeEnd: 20,
      rangeUnit: "page",
    });
    expect(result.success).toBe(false);
  });

  it("memo が 500 文字超なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2027-02-20",
      minutes: 60,
      memo: "あ".repeat(501),
    });
    expect(result.success).toBe(false);
  });
});

describe("completeStudyPlanSchema", () => {
  it("学習時間と実施範囲を通す", () => {
    expect(
      completeStudyPlanSchema.safeParse({
        minutes: 45,
        rangeStart: 10,
        rangeEnd: 20,
        rangeUnit: "page",
      }).success
    ).toBe(true);
  });

  it("学習時間が無ければ弾く", () => {
    expect(completeStudyPlanSchema.safeParse({}).success).toBe(false);
  });

  it("実施範囲が片側だけなら弾く", () => {
    expect(
      completeStudyPlanSchema.safeParse({
        minutes: 45,
        rangeStart: 10,
        rangeUnit: "page",
      }).success
    ).toBe(false);
  });
});
