import { describe, it, expect } from "vitest";
import {
  completeStudyPlanSchema,
  createStudyLogSchema,
} from "@/shared/validations/studyLog";

describe("createStudyLogSchema", () => {
  it("正常な入力を通す（時間のみ）", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 60,
      subject: "english",
    });
    expect(result.success).toBe(true);
  });

  it("正常な入力を通す（参考書＋範囲＋メモ付き）", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
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
    const result = createStudyLogSchema.safeParse({ date: "2026-02-20" });
    expect(result.success).toBe(false);
  });

  it("minutes が 0 なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 0,
    });
    expect(result.success).toBe(false);
  });

  it("minutes が負なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: -30,
    });
    expect(result.success).toBe(false);
  });

  it("minutes が 1440 ちょうどは通す（境界）", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 1440,
    });
    expect(result.success).toBe(true);
  });

  it("minutes が 1441 なら弾く（境界超え）", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 1441,
    });
    expect(result.success).toBe(false);
  });

  it("minutes が小数なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 30.5,
    });
    expect(result.success).toBe(false);
  });

  it("不正な科目は弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 60,
      subject: "cooking",
    });
    expect(result.success).toBe(false);
  });

  it("範囲が片側だけなら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 60,
      rangeStart: 10,
      rangeUnit: "page",
    });
    expect(result.success).toBe(false);
  });

  it("範囲を入れたのに単位が無ければ弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 60,
      rangeStart: 10,
      rangeEnd: 20,
    });
    expect(result.success).toBe(false);
  });

  it("開始 > 終了 なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 60,
      rangeStart: 30,
      rangeEnd: 20,
      rangeUnit: "page",
    });
    expect(result.success).toBe(false);
  });

  it("memo が 500 文字超なら弾く", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2026-02-20",
      minutes: 60,
      memo: "あ".repeat(501),
    });
    expect(result.success).toBe(false);
  });

  it("未来日は実績として記録できない", () => {
    const result = createStudyLogSchema.safeParse({
      date: "2999-01-01",
      minutes: 60,
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

describe("createStudyLogSchema の日付（JUK-75）", () => {
  const firstIssue = (date: unknown) => {
    const result = createStudyLogSchema.safeParse({ date, minutes: 30 });
    if (result.success) return null;
    const [issue] = result.error.issues;
    return { message: issue.message, code: issue.code === "custom" ? issue.params?.code : issue.code };
  };

  it("YYYY-MM-DD の過去の日付は通す（うるう日も）", () => {
    expect(firstIssue("2026-09-01")).toBeNull();
    expect(firstIssue("2024-02-29")).toBeNull();
    expect(firstIssue("2000-02-29")).toBeNull();
  });

  it.each(["1", "2026-09-3", "2026/09/01", "2026-09-01T10:00:00Z", " ", "２０２６-09-01"])(
    "形が違う %j は弾く（以前は new Date() に渡して保存していた）",
    (date) => {
      expect(firstIssue(date)).toEqual({ message: "日付は YYYY-MM-DD で指定してください", code: "invalid_format" });
    }
  );

  it.each(["2026-02-30", "2025-02-29", "1900-02-29", "2025-13-01", "2025-00-10", "2025-04-31", "2025-01-00"])(
    "暦に無い %j は弾く",
    (date) => {
      expect(firstIssue(date)).toEqual({ message: "存在しない日付です", code: "invalid_date" });
    }
  );

  it("空文字は「日付を選択してください」が先", () => {
    expect(firstIssue("")).toEqual({ message: "日付を選択してください", code: "too_small" });
  });

  it("未来日は弾く", () => {
    expect(firstIssue("2099-01-01")).toEqual({ message: "未来日は実績として記録できません", code: "future_date" });
  });
});
