import { describe, it, expect } from "vitest";
import {
  computeStreak,
  computeHeatmap,
  computeSubjectMinutes,
  type StatLog,
} from "@/lib/studyStats";

// date + minutes（+ subject）だけの実績を作るヘルパー
const l = (date: string, minutes: number, subject?: string): StatLog => ({
  date,
  minutes,
  subject,
});

describe("computeStreak", () => {
  it("達成日がなければ 0", () => {
    expect(computeStreak([], "2026-07-11")).toBe(0);
    expect(
      computeStreak([l("2026-07-11", 0), l("2026-07-10", 0)], "2026-07-11")
    ).toBe(0);
  });

  it("今日を含む連続達成日を数える", () => {
    const logs = [
      l("2026-07-11", 60),
      l("2026-07-10", 30),
      l("2026-07-09", 90),
    ];
    expect(computeStreak(logs, "2026-07-11")).toBe(3);
  });

  it("今日が未記録でも昨日までが連続なら継続を維持する", () => {
    const logs = [
      // 今日はまだ記録なし
      l("2026-07-10", 60),
      l("2026-07-09", 60),
    ];
    expect(computeStreak(logs, "2026-07-11")).toBe(2);
  });

  it("途中で途切れたらそこで止まる", () => {
    const logs = [
      l("2026-07-11", 60),
      l("2026-07-10", 60),
      // 07-09 は記録なし → ここで途切れる
      l("2026-07-08", 60),
    ];
    expect(computeStreak(logs, "2026-07-11")).toBe(2);
  });

  it("同じ日に複数の実績があっても1日として数える", () => {
    const logs = [
      l("2026-07-11", 30),
      l("2026-07-11", 30),
      l("2026-07-10", 60),
    ];
    expect(computeStreak(logs, "2026-07-11")).toBe(2);
  });

  it("月をまたいでも連続を数えられる", () => {
    const logs = [
      l("2026-08-01", 60),
      l("2026-07-31", 60),
      l("2026-07-30", 60),
    ];
    expect(computeStreak(logs, "2026-08-01")).toBe(3);
  });

  it("昨日も今日も未記録なら 0", () => {
    const logs = [l("2026-07-09", 60)];
    expect(computeStreak(logs, "2026-07-11")).toBe(0);
  });
});

describe("computeHeatmap", () => {
  const flat = (weeks: ReturnType<typeof computeHeatmap>) => weeks.flat();

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
    expect(cells.slice(0, firstDow).every((c) => c.ymd === null)).toBe(true);
    expect(cells[firstDow].ymd).toBe("2026-07-01");
  });

  it("学習時間を日付ごとに合算する", () => {
    const logs = [
      l("2026-07-10", 60),
      l("2026-07-10", 30),
      l("2026-07-11", 45),
    ];
    const cells = flat(computeHeatmap(logs, "2026-07-01"));
    expect(cells.find((c) => c.ymd === "2026-07-10")?.minutes).toBe(90);
    expect(cells.find((c) => c.ymd === "2026-07-11")?.minutes).toBe(45);
    expect(cells.find((c) => c.ymd === "2026-07-12")?.minutes).toBe(0);
  });

  it("日付ごとに科目別の構成も集計する", () => {
    const logs = [
      l("2026-07-10", 60, "english"),
      l("2026-07-10", 30, "math"),
      l("2026-07-10", 15),
    ];
    const cell = flat(computeHeatmap(logs, "2026-07-01")).find(
      (item) => item.ymd === "2026-07-10"
    );

    expect(cell?.subjects).toEqual([
      { value: "english", minutes: 60 },
      { value: "math", minutes: 30 },
      { value: "other", minutes: 15 },
    ]);
  });

  it("対象月以外の実績は集計に含めない", () => {
    const logs = [l("2026-06-30", 60), l("2026-08-01", 60)];
    const cells = flat(computeHeatmap(logs, "2026-07-15"));
    expect(cells.every((c) => c.minutes === 0)).toBe(true);
  });
});

describe("computeSubjectMinutes", () => {
  it("期間内の科目別に合計し、SUBJECTS の並びで返す", () => {
    const logs = [
      l("2026-07-06", 60, "english"),
      l("2026-07-07", 30, "english"),
      l("2026-07-08", 90, "math"),
    ];
    const result = computeSubjectMinutes(logs, "2026-07-06", "2026-07-12");
    const map = Object.fromEntries(result.map((r) => [r.value, r.minutes]));
    expect(map.english).toBe(90);
    expect(map.math).toBe(90);
    expect(map.japanese).toBe(0);
    // 先頭は SUBJECTS の並び通り english
    expect(result[0].value).toBe("english");
  });

  it("期間外の実績は含めない（両端は含む）", () => {
    const logs = [
      l("2026-07-05", 60, "english"), // 期間前
      l("2026-07-06", 60, "english"), // 開始日（含む）
      l("2026-07-12", 60, "english"), // 終了日（含む）
      l("2026-07-13", 60, "english"), // 期間後
    ];
    const result = computeSubjectMinutes(logs, "2026-07-06", "2026-07-12");
    const english = result.find((r) => r.value === "english");
    expect(english?.minutes).toBe(120);
  });

  it("科目未設定（null）は その他(other) に合算する", () => {
    const logs = [l("2026-07-06", 40, undefined), l("2026-07-06", 20, "other")];
    const result = computeSubjectMinutes(logs, "2026-07-06", "2026-07-12");
    const other = result.find((r) => r.value === "other");
    expect(other?.minutes).toBe(60);
  });
});
