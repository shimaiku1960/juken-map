import { describe, expect, it } from "vitest";
import { quickLogValues } from "./quickLog";

describe("quickLogValues", () => {
  it("読み取れた項目だけを返す", () => {
    expect(
      quickLogValues(
        { date: "2026-10-10", minutes: 90, subject: "math", textbookId: 3 },
        { dateFixed: false }
      )
    ).toEqual({ date: "2026-10-10", minutes: 90, subject: "math", textbookId: 3 });

    expect(
      quickLogValues(
        { date: null, minutes: 30, subject: null, textbookId: null },
        { dateFixed: false }
      )
    ).toEqual({ minutes: 30 });
  });

  it("何も読み取れなければ空になる", () => {
    expect(
      quickLogValues(
        { date: null, minutes: null, subject: null, textbookId: null },
        { dateFixed: false }
      )
    ).toEqual({});
  });

  it("日付を決めて開いたフォームでは日付を変えない", () => {
    expect(
      quickLogValues(
        { date: "2026-10-10", minutes: 30, subject: null, textbookId: null },
        { dateFixed: true }
      )
    ).toEqual({ minutes: 30 });
  });
});
