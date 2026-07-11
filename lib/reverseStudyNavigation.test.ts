import { describe, expect, it } from "vitest";
import {
  computeCurrentAmount,
  computeStudyNavigation,
} from "@/lib/reverseStudyNavigation";

describe("computeCurrentAmount", () => {
  it("同じ参考書・単位の最大到達位置を返す", () => {
    expect(
      computeCurrentAmount(
        1,
        "page",
        [
          {
            textbookId: 1,
            date: "2026-07-10",
            rangeEnd: 20,
            rangeUnit: "page",
          },
          {
            textbookId: 1,
            date: "2026-07-11",
            rangeEnd: 35,
            rangeUnit: "page",
          },
          {
            textbookId: 1,
            date: "2026-07-11",
            rangeEnd: 50,
            rangeUnit: "question",
          },
          {
            textbookId: 2,
            date: "2026-07-11",
            rangeEnd: 80,
            rangeUnit: "page",
          },
        ],
        "2026-07-11"
      )
    ).toBe(35);
  });

  it("対象実績がなければ0を返す", () => {
    expect(computeCurrentAmount(1, "page", [], "2026-07-11")).toBe(0);
  });

  it("未来の日付の実績は現在位置に含めない", () => {
    expect(
      computeCurrentAmount(
        1,
        "page",
        [
          {
            textbookId: 1,
            date: "2026-07-11",
            rangeEnd: 30,
            rangeUnit: "page",
          },
          {
            textbookId: 1,
            date: "2026-07-12",
            rangeEnd: 80,
            rangeUnit: "page",
          },
        ],
        "2026-07-11"
      )
    ).toBe(30);
  });
});

describe("computeStudyNavigation", () => {
  it("残量を残日数で割り、今日の範囲を返す", () => {
    expect(
      computeStudyNavigation({
        totalAmount: 300,
        currentAmount: 120,
        today: "2026-07-11",
        examDate: "2026-08-25",
      })
    ).toEqual({
      currentAmount: 120,
      remainingAmount: 180,
      remainingDays: 45,
      todayStart: 121,
      todayEnd: 124,
      dailyAmount: 4,
      status: "active",
    });
  });

  it("割り切れない残量は切り上げる", () => {
    const result = computeStudyNavigation({
      totalAmount: 10,
      currentAmount: 0,
      today: "2026-07-11",
      examDate: "2026-07-14",
    });
    expect(result.dailyAmount).toBe(4);
    expect(result.todayEnd).toBe(4);
  });

  it("完了済みならcompletedを返す", () => {
    const result = computeStudyNavigation({
      totalAmount: 100,
      currentAmount: 120,
      today: "2026-07-11",
      examDate: "2026-08-01",
    });
    expect(result.status).toBe("completed");
    expect(result.remainingAmount).toBe(0);
  });

  it("受験日当日または期限超過ならexpiredを返す", () => {
    const result = computeStudyNavigation({
      totalAmount: 100,
      currentAmount: 20,
      today: "2026-07-11",
      examDate: "2026-07-11",
    });
    expect(result.status).toBe("expired");
    expect(result.dailyAmount).toBe(0);
  });
});
