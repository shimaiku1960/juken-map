import { randomUUID } from "node:crypto";
import { afterAll, describe, expect, it } from "vitest";
import { execute, pool, select } from "@/api/infra/db";
import {
  EMAIL_GLOBAL_PER_DAY,
  EMAIL_PER_RECIPIENT_PER_HOUR,
  reserveEmailSend,
} from "@/api/infra/email-limits";
import { takeLogLines } from "@/api/test-support";

// メール送信の上限（セキュリティ基準 06 の E1）。宛先ごと・全体の両方で止まること。
//
// 全体の数は表の全行を数えるので、ほかのテストが送った行と混ざらないよう、テストごとに
// 遠い未来の時刻を「今」として渡す（窓の外になった本当の時刻の行は数えられない）。

const FUTURE = Date.UTC(2100, 0, 1);
let testSeq = 0;
/** テストごとに1か月ずつずらした「今」。 */
function freshNow() {
  testSeq += 1;
  return new Date(FUTURE + testSeq * 30 * 24 * 60 * 60 * 1000);
}

const HOUR_MS = 60 * 60 * 1000;

function address() {
  return `email-limits-${randomUUID()}@example.test`;
}

afterAll(async () => {
  await execute("DELETE FROM EmailSend WHERE sentAt >= ?", [new Date(FUTURE)]);
  await pool.end();
});

async function reserveTimes(times: number, to: string, now: Date) {
  const results: boolean[] = [];
  for (let i = 0; i < times; i += 1) results.push(await reserveEmailSend("password-reset", to, now));
  return results;
}

describe("E1 宛先ごとの上限", () => {
  it("同じ宛先へは1時間に上限の数まで送り、それを超えたら送らない", async () => {
    const now = freshNow();
    const to = address();

    const results = await reserveTimes(EMAIL_PER_RECIPIENT_PER_HOUR + 2, to, now);

    expect(results).toEqual([
      ...Array(EMAIL_PER_RECIPIENT_PER_HOUR).fill(true),
      false,
      false,
    ]);
  });

  it("確認メールと再設定メールを合わせて数え、大文字にしても同じ宛先として数える", async () => {
    const now = freshNow();
    const to = address();
    for (let i = 0; i < EMAIL_PER_RECIPIENT_PER_HOUR; i += 1) {
      expect(await reserveEmailSend(i % 2 === 0 ? "verification" : "password-reset", to, now)).toBe(true);
    }

    expect(await reserveEmailSend("verification", to.toUpperCase(), now)).toBe(false);
  });

  it("止めたことは宛先を伏せてログに残し、送らなかった分は数に入れない", async () => {
    const now = freshNow();
    const to = address();
    await reserveTimes(EMAIL_PER_RECIPIENT_PER_HOUR, to, now);
    takeLogLines();

    expect(await reserveEmailSend("verification", to, now)).toBe(false);

    const lines = JSON.stringify(takeLogLines());
    expect(lines).toContain("send limit reached");
    expect(lines).toContain('"reason":"recipient"');
    expect(lines).not.toContain(to);
    const [{ n }] = await select<{ n: number }>(
      "SELECT COUNT(*) AS n FROM EmailSend WHERE sentAt = ?",
      [now]
    );
    expect(n).toBe(EMAIL_PER_RECIPIENT_PER_HOUR);
  });

  it("ほかの宛先は止めない", async () => {
    const now = freshNow();
    await reserveTimes(EMAIL_PER_RECIPIENT_PER_HOUR, address(), now);

    expect(await reserveEmailSend("password-reset", address(), now)).toBe(true);
  });

  it("1時間たてば、また送れる", async () => {
    const now = freshNow();
    const to = address();
    await reserveTimes(EMAIL_PER_RECIPIENT_PER_HOUR, to, now);

    expect(await reserveEmailSend("password-reset", to, new Date(now.getTime() + HOUR_MS + 1))).toBe(true);
  });

  it("運営者への通知は宛先ごとの上限にかけない（登録が続いても知らせが止まらない）", async () => {
    const now = freshNow();
    const results: boolean[] = [];
    for (let i = 0; i < EMAIL_PER_RECIPIENT_PER_HOUR + 2; i += 1) {
      results.push(await reserveEmailSend("admin-new-user", "owner@example.test", now));
    }

    expect(results.every(Boolean)).toBe(true);
  });

  it("同時に送らせても、上限を超えては送らない", async () => {
    const now = freshNow();
    const to = address();

    const results = await Promise.all(
      Array.from({ length: EMAIL_PER_RECIPIENT_PER_HOUR + 5 }, () => reserveEmailSend("password-reset", to, now))
    );

    expect(results.filter(Boolean).length).toBeLessThanOrEqual(EMAIL_PER_RECIPIENT_PER_HOUR);
    expect(results.filter(Boolean).length).toBeGreaterThan(0);
  });
});

describe("E1 全体の上限", () => {
  it("24時間で上限の数まで送り、それを超えたら宛先が違っても送らない", async () => {
    const now = freshNow();
    for (let i = 0; i < EMAIL_GLOBAL_PER_DAY; i += 1) {
      expect(await reserveEmailSend("verification", address(), now)).toBe(true);
    }
    takeLogLines();

    expect(await reserveEmailSend("verification", address(), now)).toBe(false);
    // 運営者への通知も全体の数に入る。
    expect(await reserveEmailSend("admin-new-user", "owner@example.test", now)).toBe(false);
    expect(JSON.stringify(takeLogLines())).toContain('"reason":"global"');
  });

  it("24時間より前の分は数えず、送るときに消す", async () => {
    const now = freshNow();
    for (let i = 0; i < EMAIL_GLOBAL_PER_DAY; i += 1) {
      await reserveEmailSend("verification", address(), now);
    }
    const nextDay = new Date(now.getTime() + 24 * HOUR_MS + 1);

    expect(await reserveEmailSend("verification", address(), nextDay)).toBe(true);
    const [{ n }] = await select<{ n: number }>(
      "SELECT COUNT(*) AS n FROM EmailSend WHERE sentAt = ?",
      [now]
    );
    // 1回の送信で消すのは100行まで。上限が80通なので1回で消し切れる。
    expect(n).toBe(0);
  });
});
