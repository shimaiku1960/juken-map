import { describe, expect, it } from "vitest";
import { buildExamSchedule, type ExamScheduleInput } from "@/lib/examSchedule";

const make = (
  id: number,
  examDate: string,
  overrides: Partial<ExamScheduleInput> = {}
): ExamScheduleInput => ({
  id,
  universityName: `大学${id}`,
  facultyName: `学部${id}`,
  examDate,
  isFirstChoice: false,
  tags: [],
  ...overrides,
});

describe("buildExamSchedule", () => {
  it("受験日の早い順に並べる", () => {
    const result = buildExamSchedule([
      make(1, "2027-02-20"),
      make(2, "2027-02-10"),
      make(3, "2027-02-15"),
    ]);
    expect(result.map((e) => e.id)).toEqual([2, 3, 1]);
  });

  it("重複・連戦がなければ sameDayCount=0・backToBack=false", () => {
    const result = buildExamSchedule([
      make(1, "2027-02-10"),
      make(2, "2027-02-15"),
    ]);
    expect(result.every((e) => e.sameDayCount === 0)).toBe(true);
    expect(result.every((e) => e.backToBack === false)).toBe(true);
  });

  it("同日受験を sameDayCount に数える（自分は除く）", () => {
    const result = buildExamSchedule([
      make(1, "2027-02-10"),
      make(2, "2027-02-10"),
      make(3, "2027-02-10"),
    ]);
    expect(result.every((e) => e.sameDayCount === 2)).toBe(true);
  });

  it("前日または翌日に別の受験があれば backToBack=true", () => {
    const result = buildExamSchedule([
      make(1, "2027-02-10"),
      make(2, "2027-02-11"),
      make(3, "2027-02-20"),
    ]);
    const byId = new Map(result.map((e) => [e.id, e]));
    expect(byId.get(1)?.backToBack).toBe(true);
    expect(byId.get(2)?.backToBack).toBe(true);
    expect(byId.get(3)?.backToBack).toBe(false);
  });

  it("空配列は空を返す", () => {
    expect(buildExamSchedule([])).toEqual([]);
  });

  it("時刻が違っても同じ暦日は同日扱い", () => {
    const result = buildExamSchedule([
      make(1, "2027-02-10T01:00:00"),
      make(2, "2027-02-10T23:00:00"),
    ]);
    expect(result.every((e) => e.sameDayCount === 1)).toBe(true);
  });
});
