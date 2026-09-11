import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";
import { getResend } from "@/api/infra/resend";
import { pushLineText } from "@/api/infra/line";
import { tokyoDateRange } from "@/api/domain/dailyNotification";
import { sendDailyNotifications } from "@/api/services/sendDailyNotifications";
import {
  cleanup,
  createLineConnection,
  createNotificationPreference,
  createStudyLog,
  createStudyPlan,
  createTextbook,
  createUser,
  findNotificationDeliveries,
} from "../test-db/fixtures.ts";

// 外部サービス（メール・LINE）だけを差し替え、DB は本物のテスト用 MySQL を使う。
vi.mock("@/api/infra/resend", () => ({ getResend: vi.fn() }));
vi.mock("@/api/infra/line", () => ({ pushLineText: vi.fn() }));

// sendDailyNotifications は DB 上の全ユーザーから対象を探すので、並列に走る別のテストが
// 作ったユーザーも拾う。結果の件数ではなく「このテストで作ったユーザーに何が起きたか」
// （送信の呼び出しと配信記録）で確かめる。
const now = new Date("2026-08-30T12:00:00.000Z"); // 日本時間 8/30 21:00
const { start, end } = tokyoDateRange(now);
const send = vi.fn();

const sentTo = (email: string) =>
  send.mock.calls.filter(([payload]) => payload.to === email).map(([payload]) => payload);

beforeEach(() => {
  vi.clearAllMocks();
  send.mockResolvedValue({ data: { id: "mail-1" }, error: null });
  vi.mocked(getResend).mockReturnValue({
    emails: { send },
  } as unknown as ReturnType<typeof getResend>);
});

afterAll(cleanup);

describe("sendDailyNotifications", () => {
  it("有効なユーザーへ送り、配信履歴を残す", async () => {
    const user = await createUser();
    await createNotificationPreference(user.id, { eveningEnabled: true });
    await createStudyPlan(user.id, { date: start, done: true });
    await createStudyPlan(user.id, { date: start });
    await createStudyLog(user.id, { date: start, minutes: 30 });
    await createStudyLog(user.id, { date: start, minutes: 15 });

    const result = await sendDailyNotifications("evening", now);

    expect(result.sent).toBeGreaterThanOrEqual(1);
    const [mail] = sentTo(user.session.user.email);
    // 予定と実績を1本の JOIN で取ると（予定2 × 実績2）の4行になり、90分・4件と数えてしまう
    expect(mail.text).toContain("今日は45分の学習を記録しました。");
    expect(mail.text).toContain("予定2件中1件を完了しました。");
    expect(await findNotificationDeliveries(user.id)).toEqual([
      expect.objectContaining({ date: start, slot: "evening", channel: "email" }),
    ]);
  });

  it("通知をオフにしているユーザー・設定の無いユーザーには送らない", async () => {
    const off = await createUser();
    await createNotificationPreference(off.id, { morningEnabled: true, eveningEnabled: false });
    const noPreference = await createUser();

    await sendDailyNotifications("evening", now);

    expect(sentTo(off.session.user.email)).toHaveLength(0);
    expect(sentTo(noPreference.session.user.email)).toHaveLength(0);
    expect(await findNotificationDeliveries(off.id)).toHaveLength(0);
  });

  it("本文にはその日の予定と実績だけを使う", async () => {
    const user = await createUser();
    await createNotificationPreference(user.id, { morningEnabled: true });
    const textbookId = await createTextbook(user.id, { name: "青チャート" });
    await createStudyPlan(user.id, { date: start, textbookId });
    await createStudyPlan(user.id, { date: new Date(end.getTime() - 1), content: "今日の終わり" });
    await createStudyPlan(user.id, { date: end, content: "翌日の予定" });
    await createStudyPlan(user.id, { date: new Date(start.getTime() - 1), content: "前日の予定" });

    await sendDailyNotifications("morning", now);

    const [mail] = sentTo(user.session.user.email);
    expect(mail.text).toContain("今日の予定は2件です。");
    expect(mail.text).toContain("青チャート");
    expect(mail.text).toContain("今日の終わり");
    expect(mail.text).not.toContain("翌日の予定");
    expect(mail.text).not.toContain("前日の予定");
  });

  it("同じ日・同じ時間帯の2回目は送らない", async () => {
    const user = await createUser();
    await createNotificationPreference(user.id, { morningEnabled: true });

    await sendDailyNotifications("morning", now);
    const second = await sendDailyNotifications("morning", now);

    expect(second.skipped).toBeGreaterThanOrEqual(1);
    expect(sentTo(user.session.user.email)).toHaveLength(1);
    expect(await findNotificationDeliveries(user.id)).toHaveLength(1);
  });

  it("送信に失敗したら配信履歴を消し、次の実行で再び送れるようにする", async () => {
    const user = await createUser();
    await createNotificationPreference(user.id, { morningEnabled: true });
    send.mockImplementation(async (payload) =>
      payload.to === user.session.user.email
        ? { data: null, error: { message: "unavailable" } }
        : { data: { id: "mail-1" }, error: null }
    );

    const failed = await sendDailyNotifications("morning", now);

    expect(failed.failed).toBeGreaterThanOrEqual(1);
    expect(await findNotificationDeliveries(user.id)).toHaveLength(0);

    send.mockResolvedValue({ data: { id: "mail-2" }, error: null });
    await sendDailyNotifications("morning", now);

    expect(sentTo(user.session.user.email)).toHaveLength(2);
    expect(await findNotificationDeliveries(user.id)).toHaveLength(1);
  });

  it("メールとLINEをそれぞれ独立して送信する", async () => {
    const user = await createUser();
    await createNotificationPreference(user.id, { morningEnabled: true, lineMorningEnabled: true });
    const lineUserId = await createLineConnection(user.id);

    await sendDailyNotifications("morning", now);

    expect(sentTo(user.session.user.email)).toHaveLength(1);
    expect(pushLineText).toHaveBeenCalledWith(lineUserId, expect.stringContaining("おはようございます"));
    expect((await findNotificationDeliveries(user.id)).map((d) => d.channel)).toEqual(["email", "line"]);
  });

  it("LINE通知だけオンでも、未連携ならLINEには送らない", async () => {
    const user = await createUser();
    await createNotificationPreference(user.id, { lineMorningEnabled: true });

    await sendDailyNotifications("morning", now);

    expect(sentTo(user.session.user.email)).toHaveLength(0);
    expect(await findNotificationDeliveries(user.id)).toHaveLength(0);
  });
});
