import { execute, isDuplicateEntry, select } from "@/api/infra/db";
import { getResend } from "@/api/infra/resend";
import { pushLineText } from "@/api/infra/line";
import { buildDailyNotification, tokyoDateRange, type NotificationSlot } from "@/api/domain/dailyNotification";

const FROM = "受験マップ <noreply@juken-map.com>";
type Channel = "email" | "line";

// 列名は ? で渡せない（値ではなく識別子なので）。利用者の入力ではなく、
// この対応表の固定の名前だけを SQL に埋め込む。
const SLOT_COLUMNS = {
  morning: { email: "morningEnabled", line: "lineMorningEnabled" },
  evening: { email: "eveningEnabled", line: "lineEveningEnabled" },
} as const;

type RecipientRow = {
  id: string;
  email: string | null;
  name: string | null;
  nickname: string | null;
  morningEnabled: boolean;
  eveningEnabled: boolean;
  lineMorningEnabled: boolean;
  lineEveningEnabled: boolean;
  lineUserId: string | null;
};

type PlanRow = {
  userId: string;
  done: boolean;
  content: string | null;
  subject: string | null;
  textbookName: string | null;
};

/**
 * その時間帯に通知を受け取る設定のユーザーと、その日の予定・実績を集める。
 *
 * SQL は3本に分ける。ユーザーから見て「予定」と「実績」はどちらも1対多なので、
 * 1本の JOIN にすると（予定の数 × 実績の数）の行に膨らみ、学習時間が重複して数えられる。
 * 予定と実績はそれぞれ別に取り、ユーザー ID でまとめてから組み合わせる。
 * Prisma の入れ子の select も、中ではこれと同じく別々の SQL に分けていた。
 */
export async function findRecipients(slot: NotificationSlot, start: Date, end: Date) {
  const columns = SLOT_COLUMNS[slot];
  // 通知設定の無いユーザーは対象外なので、設定とは内部結合（JOIN）。
  // LINE は未連携でもメールだけ受け取れるので、外部結合（LEFT JOIN）。
  const users = await select<RecipientRow>(
    `SELECT u.id, u.email, u.name, u.nickname,
            np.morningEnabled, np.eveningEnabled, np.lineMorningEnabled, np.lineEveningEnabled,
            lc.lineUserId
     FROM \`user\` AS u
     JOIN NotificationPreference AS np ON np.userId = u.id
     LEFT JOIN LineConnection AS lc ON lc.userId = u.id
     WHERE np.${columns.email} = TRUE OR np.${columns.line} = TRUE
     ORDER BY u.id`
  );
  // IN () は空だと SQL の構文エラーになるので、先に抜ける。
  if (users.length === 0) return [];

  const userIds = users.map((user) => user.id);
  const plans = await select<PlanRow>(
    `SELECT p.userId, p.done, p.content, p.subject, t.name AS textbookName
     FROM StudyPlan AS p
     LEFT JOIN Textbook AS t ON t.id = p.textbookId
     WHERE p.userId IN (?) AND p.date >= ? AND p.date < ?
     ORDER BY p.id`,
    [userIds, start, end]
  );
  const logs = await select<{ userId: string; minutes: number }>(
    `SELECT userId, minutes FROM StudyLog
     WHERE userId IN (?) AND date >= ? AND date < ?
     ORDER BY id`,
    [userIds, start, end]
  );

  return users.map((user) => ({
    id: user.id,
    email: user.email,
    name: user.name,
    nickname: user.nickname,
    notificationPreference: {
      morningEnabled: user.morningEnabled,
      eveningEnabled: user.eveningEnabled,
      lineMorningEnabled: user.lineMorningEnabled,
      lineEveningEnabled: user.lineEveningEnabled,
    },
    lineConnection: user.lineUserId === null ? null : { lineUserId: user.lineUserId },
    studyPlans: plans
      .filter((plan) => plan.userId === user.id)
      .map((plan) => ({
        done: plan.done,
        content: plan.content,
        subject: plan.subject,
        textbook: plan.textbookName === null ? null : { name: plan.textbookName },
      })),
    studyLogs: logs
      .filter((log) => log.userId === user.id)
      .map((log) => ({ minutes: log.minutes })),
  }));
}

export async function sendDailyNotifications(slot: NotificationSlot, now = new Date()) {
  const { date, start, end } = tokyoDateRange(now);
  const users = await findRecipients(slot, start, end);

  let eligible = 0;
  let sent = 0;
  let skipped = 0;
  let failed = 0;

  for (const user of users) {
    const message = buildDailyNotification({
      slot,
      nickname: user.nickname ?? user.name ?? "ユーザー",
      plans: user.studyPlans,
      logs: user.studyLogs,
    });
    const channels: Channel[] = [];
    const emailEnabled = slot === "morning" ? user.notificationPreference?.morningEnabled : user.notificationPreference?.eveningEnabled;
    const lineEnabled = slot === "morning" ? user.notificationPreference?.lineMorningEnabled : user.notificationPreference?.lineEveningEnabled;
    if (emailEnabled && user.email) channels.push("email");
    if (lineEnabled && user.lineConnection) channels.push("line");

    for (const channel of channels) {
      eligible += 1;
      let deliveryId: number | null = null;
      try {
        // 送る前に「この日・この時間帯・この経路は送った」印を先に入れる。
        // 同じ組み合わせには UNIQUE 制約があるので、2回目は重複エラーになり、送らずに済む。
        const delivery = await execute(
          `INSERT INTO NotificationDelivery (userId, date, slot, channel, createdAt)
           VALUES (?, ?, ?, ?, ?)`,
          [user.id, start, slot, channel, new Date()]
        );
        deliveryId = delivery.insertId;
      } catch (error) {
        if (isDuplicateEntry(error)) {
          skipped += 1;
          continue;
        }
        throw error;
      }

      try {
        if (channel === "email") {
          const { error } = await getResend().emails.send({ from: FROM, to: user.email!, ...message });
          if (error) throw new Error(error.message);
        } else {
          await pushLineText(user.lineConnection!.lineUserId, message.text);
        }
        sent += 1;
      } catch (error) {
        failed += 1;
        // 送れなかったので印を消し、次の実行で再び送れるようにする。
        await execute("DELETE FROM NotificationDelivery WHERE id = ?", [deliveryId]);
        console.error(`[daily-notification] ${slot}/${channel} delivery failed for user ${user.id}.`, error);
      }
    }
  }

  return { date, slot, eligible, sent, skipped, failed };
}
