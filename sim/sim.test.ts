import { describe, expect, it } from "vitest";
import { mergeCookies } from "./client";
import { dayPlanFor, personaFor, signupHours, HOUR_WEIGHTS } from "./persona";
import { extractVerificationPath } from "./resend-inbox";
import { dayNumber, dueVisitors, planDays, signupSlots, visitorsOn } from "./schedule";

const SEED = 20260918;

describe("persona", () => {
  it("同じ連番なら毎回同じ人になる（ランナーが状態を持たなくてよい）", () => {
    expect(personaFor(42, SEED)).toEqual(personaFor(42, SEED));
    expect(dayPlanFor(personaFor(42, SEED), 3, 20000, 2, SEED)).toEqual(
      dayPlanFor(personaFor(42, SEED), 3, 20000, 2, SEED)
    );
  });

  it("続き方の型がおおむね想定の割合で混ざる", () => {
    const counts: Record<string, number> = {};
    for (let seq = 1; seq <= 4000; seq++) {
      const { cohort } = personaFor(seq, SEED);
      counts[cohort] = (counts[cohort] ?? 0) + 1;
    }
    expect(counts.steady / 4000).toBeCloseTo(0.2, 1);
    expect(counts.fading / 4000).toBeCloseTo(0.35, 1);
    expect(counts.sporadic / 4000).toBeCloseTo(0.3, 1);
    expect(counts.dropped / 4000).toBeCloseTo(0.15, 1);
  });

  it("登録した日は必ず開く", () => {
    for (let seq = 1; seq <= 200; seq++) {
      expect(dayPlanFor(personaFor(seq, SEED), 0, 20000, 3, SEED).active).toBe(true);
    }
  });

  it("重みが0の時間（深夜2〜5時）には登録しない", () => {
    const hours = signupHours(1000, 20000, SEED);
    expect(hours).toHaveLength(1000);
    for (const hour of hours) expect(HOUR_WEIGHTS[hour]).toBeGreaterThan(0);
  });
});

describe("extractVerificationPath", () => {
  it("確認メールの本文からパスとクエリだけを取り出す（&amp; も戻す）", () => {
    const html =
      '<p><a href="https://juken-map.com/api/auth/verify-email?token=abc.def&amp;callbackURL=%2Fdashboard">確認</a></p>';
    expect(extractVerificationPath(html)).toBe(
      "/api/auth/verify-email?token=abc.def&callbackURL=%2Fdashboard"
    );
  });

  it("リンクが無ければ例外", () => {
    expect(() => extractVerificationPath("<p>no link</p>")).toThrow();
  });
});

describe("mergeCookies", () => {
  it("Set-Cookie の値で上書きし、Max-Age=0 は消す", () => {
    const merged = mergeCookies("a=1; b=2", [
      "b=3; Path=/; HttpOnly",
      "c=4; Path=/",
      "a=; Max-Age=0; Path=/",
    ]);
    expect(merged).toBe("b=3; c=4");
  });
});

describe("planDays", () => {
  const today = dayNumber("2026-09-18");

  it("まっさらなら、登録は時刻の早い順に連番1から振られ、翌日の来訪に数えられる", () => {
    const [first, second] = planDays({ users: [], nextSeq: 1, from: today, days: 2, signupsPerDay: 5, baseSeed: SEED });
    const signups = first.events.filter((e) => e.kind === "signup");
    expect(signups.map((e) => e.hour)).toEqual(signupSlots(5, today, SEED));
    expect(signups.map((e) => e.seq).sort()).toEqual([1, 2, 3, 4, 5]);
    expect(signups.every((e) => !e.done)).toBe(true);
    expect(first.events.some((e) => e.kind === "visit")).toBe(false);

    // 翌日の新規登録は6番から。来訪は前日に登録した人の中から選ばれる。
    const nextSignups = second.events.filter((e) => e.kind === "signup");
    expect(Math.min(...nextSignups.map((e) => e.seq))).toBe(6);
    for (const visit of second.events.filter((e) => e.kind === "visit")) {
      expect(visit.seq).toBeLessThanOrEqual(5);
    }
  });

  it("今日すでに登録・来訪した人は「済」になり、連番はその続きから振られる", () => {
    const users = [
      { seq: 1, createdAt: "2026-09-17T12:00:00+09:00", dormantFrom: null, lastActedOn: "2026-09-18" },
      { seq: 2, createdAt: "2026-09-18T09:00:00Z", dormantFrom: null, lastActedOn: "2026-09-18" },
    ];
    const [day] = planDays({ users, nextSeq: 3, from: today, days: 1, signupsPerDay: 3, baseSeed: SEED });
    const signups = day.events.filter((e) => e.kind === "signup").sort((a, b) => a.seq - b.seq);
    expect(signups.map((e) => [e.seq, e.done])).toEqual([
      [2, true],
      [3, false],
      [4, false],
    ]);
    // 来訪の顔ぶれと時間は、毎時の実行が使う visitorsOn と同じ。
    const visits = day.events.filter((e) => e.kind === "visit");
    expect(visits.map((e) => [e.seq, e.hour])).toEqual(
      visitorsOn(users, today, SEED).map((v) => [v.seq, v.plan.hour])
    );
    for (const visit of visits) expect(visit.done).toBe(true);
  });

  it("来なくなった人は予定に出ない", () => {
    const users = [
      { seq: 1, createdAt: "2026-09-01T12:00:00+09:00", dormantFrom: "2026-09-10", lastActedOn: null },
    ];
    const [day] = planDays({ users, nextSeq: 2, from: today, days: 1, signupsPerDay: 0, baseSeed: SEED });
    expect(day.events).toEqual([]);
  });
});

describe("dueVisitors", () => {
  const today = dayNumber("2026-09-19");
  // 前日までに登録した100人。来訪する人・時間はペルソナで決まる。
  const users = Array.from({ length: 100 }, (_, i) => ({
    seq: i + 1,
    createdAt: "2026-09-10T12:00:00+09:00",
    dormantFrom: null,
    lastActedOn: null as string | null,
  }));
  const visits = visitorsOn(users, today, SEED);

  it("予定の時間を過ぎた人も拾う（定時実行が飛ばされた回の分）", () => {
    const due = dueVisitors(users, today, 23, SEED);
    expect(due.map((v) => v.seq).sort((a, b) => a - b)).toEqual(visits.map((v) => v.seq).sort((a, b) => a - b));
    expect(due.map((v) => v.plan.hour)).toEqual([...due.map((v) => v.plan.hour)].sort((a, b) => a - b));
  });

  it("まだ来ていない時間の人は拾わない", () => {
    const hour = 12;
    const due = dueVisitors(users, today, hour, SEED);
    expect(due.length).toBe(visits.filter((v) => v.plan.hour <= hour).length);
    for (const v of due) expect(v.plan.hour).toBeLessThanOrEqual(hour);
  });

  it("今日すでに動いた人は拾わない（前日に動いた人は拾う）", () => {
    const [acted, actedYesterday] = visits;
    const marked = users.map((u) =>
      u.seq === acted.seq
        ? { ...u, lastActedOn: "2026-09-19" }
        : u.seq === actedYesterday.seq
          ? { ...u, lastActedOn: "2026-09-18" }
          : u
    );
    const seqs = dueVisitors(marked, today, 23, SEED).map((v) => v.seq);
    expect(seqs).not.toContain(acted.seq);
    expect(seqs).toContain(actedYesterday.seq);
  });
});
