import { describe, it, expect } from "vitest";
import {
  computeStreak,
  computeHeatmap,
  type StatPlan,
} from "@/lib/studyStats";

// date + done だけのプランを作るヘルパー
const p = (date: string, done: boolean): StatPlan => ({ date, done });

describe("computeStreak", () => {
  it("達成日がなければ 0", () => {
    expect(computeStreak([], "2026-07-11")).toBe(0);
    expect(
      computeStreak([p("2026-07-11", false), p("2026-07-10", false)], "2026-07-11")
    ).toBe(0);
  });

  it("今日を含む連続達成日を数える", () => {
    const plans = [
      p("2026-07-11", true),
      p("2026-07-10", true),
      p("2026-07-09", true),
    ];
    expect(computeStreak(plans, "2026-07-11")).toBe(3);
  });

  it("今日が未達成でも昨日までが連続なら継続を維持する", () => {
    const plans = [
      p("2026-07-11", false), // 今日はまだ
      p("2026-07-10", true),
      p("2026-07-09", true),
    ];
    expect(computeStreak(plans, "2026-07-11")).toBe(2);
  });

  it("途中で途切れたらそこで止まる", () => {
    const plans = [
      p("2026-07-11", true),
      p("2026-07-10", true),
      // 07-09 は達成なし → ここで途切れる
      p("2026-07-08", true),
    ];
    expect(computeStreak(plans, "2026-07-11")).toBe(2);
  });

  it("同じ日に done が複数あっても1日として数える", () => {
    const plans = [
      p("2026-07-11", true),
      p("2026-07-11", true),
      p("2026-07-10", true),
    ];
    expect(computeStreak(plans, "2026-07-11")).toBe(2);
  });

  it("月をまたいでも連続を数えられる", () => {
    const plans = [
      p("2026-08-01", true),
      p("2026-07-31", true),
      p("2026-07-30", true),
    ];
    expect(computeStreak(plans, "2026-08-01")).toBe(3);
  });

  it("昨日も今日も未達成なら 0", () => {
    const plans = [
      p("2026-07-11", false),
      p("2026-07-10", false),
      p("2026-07-09", true),
    ];
    expect(computeStreak(plans, "2026-07-11")).toBe(0);
  });
});

describe("computeHeatmap", () => {
  const flat = (weeks: { ymd: string | null; count: number }[][]) =>
    weeks.flat();

  it("全セル数は7の倍数で、日付セルはその月の日数ぶんある", () => {
    const weeks = computeHeatmap([], "2026-07-15");
    const cells = flat(weeks);
    expect(cells.length % 7).toBe(0);
    expect(cells.filter((c) => c.ymd !== null).length).toBe(31); // 7月は31日
  });

  it("月初の空白は1日の曜日ぶん入る", () => {
    const weeks = computeHeatmap([], "2026-07-01");
    const cells = flat(weeks);
    const firstDow = new Date(2026, 6, 1).getDay();
    // 先頭 firstDow 個が空白、その次が 7/1
    expect(cells.slice(0, firstDow).every((c) => c.ymd === null)).toBe(true);
    expect(cells[firstDow].ymd).toBe("2026-07-01");
  });

  it("done 件数を日付ごとに数える（未達成は数えない）", () => {
    const plans = [
      p("2026-07-10", true),
      p("2026-07-10", true),
      p("2026-07-10", false), // 未達成は無視
      p("2026-07-11", true),
    ];
    const cells = flat(computeHeatmap(plans, "2026-07-01"));
    expect(cells.find((c) => c.ymd === "2026-07-10")?.count).toBe(2);
    expect(cells.find((c) => c.ymd === "2026-07-11")?.count).toBe(1);
    expect(cells.find((c) => c.ymd === "2026-07-12")?.count).toBe(0);
  });

  it("対象月以外の done は集計に含めない", () => {
    const plans = [p("2026-06-30", true), p("2026-08-01", true)];
    const cells = flat(computeHeatmap(plans, "2026-07-15"));
    expect(cells.every((c) => c.count === 0)).toBe(true);
  });
});
