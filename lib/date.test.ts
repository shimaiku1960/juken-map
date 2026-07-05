import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { daysUntil, formatExamDate } from "./date";

describe("daysUntil", () => {
  // 各テストの前に「今日」を 2027-01-01 に固定する
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2027-01-01T09:00:00"));
  });

  // 固定を解除して本物の時計に戻す（後続テストへの汚染防止）
  afterEach(() => {
    vi.useRealTimers();
  });

  it("未来の日付は正の日数を返す", () => {
    expect(daysUntil("2027-01-11")).toBe(10);
  });

  it("今日は0を返す", () => {
    expect(daysUntil("2027-01-01")).toBe(0);
  });

  it("過去の日付は負の日数を返す", () => {
    expect(daysUntil("2026-12-25")).toBe(-7);
  });
});

describe("formatExamDate", () => {
    it("和暦形式（YYYY年M月D日）で返す", () => {
      expect(formatExamDate("2027-02-20")).toBe("2027年2月20日");
    });
  
    it("月・日はゼロ埋めしない", () => {
      expect(formatExamDate("2027-01-05")).toBe("2027年1月5日");
    });
  });