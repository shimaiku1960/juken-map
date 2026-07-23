import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  daysUntil,
  formatExamDate,
  ymdLocal,
  todayYmd,
  todayYmdTokyo,
  ymdAfterDays,
} from "./date";

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

describe("ymdLocal", () => {
  // Date はローカル要素（年, 月index, 日）で組み立てる＝TZに依存せず日付が確定する
  it("月・日を2桁ゼロ埋めして YYYY-MM-DD を返す", () => {
    expect(ymdLocal(new Date(2027, 0, 9))).toBe("2027-01-09");
  });

  it("2桁の月・日はそのまま返す", () => {
    expect(ymdLocal(new Date(2027, 11, 25))).toBe("2027-12-25");
  });

  it("文字列を渡しても YYYY-MM-DD を返す", () => {
    // 正午指定なら全TZで同じ日付になる（境界のズレを避ける）
    expect(ymdLocal("2027-03-05T12:00:00")).toBe("2027-03-05");
  });
});

describe("todayYmd / ymdAfterDays", () => {
  // 「今日」を 2027-01-01 に固定（ローカル要素で作るのでTZ非依存）
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2027, 0, 1, 9, 0, 0));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("todayYmd は固定した今日を返す", () => {
    expect(todayYmd()).toBe("2027-01-01");
  });

  it("ymdAfterDays(0) は今日", () => {
    expect(ymdAfterDays(0)).toBe("2027-01-01");
  });

  it("ymdAfterDays(3) は3日後", () => {
    expect(ymdAfterDays(3)).toBe("2027-01-04");
  });

  it("ymdAfterDays(-1) は前日（年をまたいでも正しい）", () => {
    expect(ymdAfterDays(-1)).toBe("2026-12-31");
  });

  it("月をまたぐ加算も正しい", () => {
    vi.setSystemTime(new Date(2027, 0, 30, 9, 0, 0)); // 1/30
    expect(ymdAfterDays(3)).toBe("2027-02-02");
  });
});

describe("todayYmdTokyo", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("UTCでは前日でも日本時間の今日を返す", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2027-01-01T15:30:00.000Z"));

    expect(todayYmdTokyo()).toBe("2027-01-02");
  });
});
