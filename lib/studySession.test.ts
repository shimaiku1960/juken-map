import { describe, expect, it, vi } from "vitest";
import {
  elapsedStudyMs,
  formatStudyElapsed,
  parseStoredStudySession,
  pauseStudySession,
  recordedMinutes,
  resumeStudySession,
  reviewStudySession,
  startStudySession,
  studySessionStorageKey,
} from "@/lib/studySession";

const target = {
  planId: 1,
  label: "英単語",
  subject: "english",
  textbookId: 2,
  rangeStart: 1,
  rangeEnd: 10,
  rangeUnit: "page",
};

describe("studySession", () => {
  it("時刻差から経過時間を算出する", () => {
    vi.spyOn(Math, "random").mockReturnValue(0.5);
    const session = startStudySession(target, 1_000);
    expect(elapsedStudyMs(session, 6_500)).toBe(5_500);
  });

  it("一時停止中の時間を加算せず、再開後だけ加算する", () => {
    const started = startStudySession(target, 1_000);
    const paused = pauseStudySession(started, 6_000);
    expect(elapsedStudyMs(paused, 20_000)).toBe(5_000);

    const resumed = resumeStudySession(paused, 20_000);
    expect(elapsedStudyMs(resumed, 23_000)).toBe(8_000);
  });

  it("終了確認へ移ると計測を止める", () => {
    const session = reviewStudySession(startStudySession(target, 1_000), 4_000);
    expect(session.status).toBe("reviewing");
    expect(session.runningSince).toBeNull();
    expect(session.accumulatedMs).toBe(3_000);
  });

  it("保存分数は切り上げ、1分未満でも1分にする", () => {
    expect(recordedMinutes(startStudySession(target, 1_000), 1_001)).toBe(1);
    expect(recordedMinutes(startStudySession(target, 1_000), 62_000)).toBe(2);
  });

  it("表示用の経過時間をhh:mm:ssへ整形する", () => {
    expect(formatStudyElapsed(3_661_000)).toBe("01:01:01");
  });

  it("保存値を検証し、壊れた値は復元しない", () => {
    const session = startStudySession(target, 1_000);
    expect(parseStoredStudySession(JSON.stringify(session))).toEqual(session);
    expect(parseStoredStudySession("not-json")).toBeNull();
    expect(parseStoredStudySession(JSON.stringify({ version: 1 }))).toBeNull();
  });

  it("タイマー保存先をユーザーごとに分離する", () => {
    expect(studySessionStorageKey("user-a")).not.toBe(
      studySessionStorageKey("user-b")
    );
  });
});
