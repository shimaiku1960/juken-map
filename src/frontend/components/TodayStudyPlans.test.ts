import { describe, expect, it } from "vitest";
import { todayPlanState, type TodayPlan } from "./TodayStudyPlans";

const plan = (overrides: Partial<TodayPlan> = {}): TodayPlan => ({
  id: 1,
  label: "英単語 1〜30",
  done: false,
  recordedMinutes: null,
  ...overrides,
});

describe("todayPlanState", () => {
  it("未完了の予定があれば学習開始状態にする", () => {
    expect(todayPlanState([plan(), plan({ id: 2, done: true })], false)).toBe("active");
  });

  it("予定がなければ予定作成状態にする", () => {
    expect(todayPlanState([], false)).toBe("empty");
  });

  it("すべて完了していれば記録確認状態にする", () => {
    expect(todayPlanState([plan({ done: true })], false)).toBe("complete");
  });

  it("取得エラーを他の状態より優先する", () => {
    expect(todayPlanState([plan()], true)).toBe("error");
  });
});
