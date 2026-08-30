import { describe, expect, it } from "vitest";
import {
  buildDailyNotification,
  tokyoDateRange,
} from "@/lib/dailyNotification";

describe("tokyoDateRange", () => {
  it("UTCの日付ではなく日本時間の一日を返す", () => {
    const range = tokyoDateRange(new Date("2026-08-30T16:00:00.000Z"));

    expect(range.date).toBe("2026-08-31");
    expect(range.start.toISOString()).toBe("2026-08-30T15:00:00.000Z");
    expect(range.end.toISOString()).toBe("2026-08-31T15:00:00.000Z");
  });
});

describe("buildDailyNotification", () => {
  it("朝は今日の予定を具体的に伝える", () => {
    const message = buildDailyNotification({
      slot: "morning",
      nickname: "育朗",
      plans: [
        {
          done: false,
          content: null,
          subject: "english",
          textbook: { name: "英単語帳" },
        },
      ],
      logs: [],
    });

    expect(message.subject).toContain("今日の学習予定");
    expect(message.html).toContain("今日の予定は1件");
    expect(message.html).toContain("英単語帳");
  });

  it("夜は学習時間と予定達成数を伝える", () => {
    const message = buildDailyNotification({
      slot: "evening",
      nickname: "育朗",
      plans: [
        { done: true, content: "過去問", subject: null, textbook: null },
        { done: false, content: "復習", subject: null, textbook: null },
      ],
      logs: [{ minutes: 25 }, { minutes: 35 }],
    });

    expect(message.html).toContain("60分");
    expect(message.html).toContain("予定2件中1件");
  });

  it("実績0でも責めない文面にする", () => {
    const message = buildDailyNotification({
      slot: "evening",
      nickname: "<ユーザー>",
      plans: [],
      logs: [],
    });

    expect(message.html).toContain("短い時間でも、記録から再開できます");
    expect(message.html).toContain("&lt;ユーザー&gt;");
  });
});
