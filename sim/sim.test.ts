import { describe, expect, it } from "vitest";
import { mergeCookies } from "./client";
import { dayPlanFor, personaFor, signupHours, HOUR_WEIGHTS } from "./persona";
import { extractVerificationPath } from "./resend-inbox";

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
