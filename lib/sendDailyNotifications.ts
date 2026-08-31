import prisma from "@/lib/prisma";
import { getResend } from "@/lib/resend";
import { pushLineText } from "@/lib/line";
import { buildDailyNotification, tokyoDateRange, type NotificationSlot } from "@/lib/dailyNotification";

const FROM = "受験マップ <noreply@juken-map.com>";
type Channel = "email" | "line";

function isUniqueConstraintError(error: unknown) {
  return typeof error === "object" && error !== null && "code" in error && error.code === "P2002";
}

export async function sendDailyNotifications(slot: NotificationSlot, now = new Date()) {
  const { date, start, end } = tokyoDateRange(now);
  const emailField = slot === "morning" ? "morningEnabled" : "eveningEnabled";
  const lineField = slot === "morning" ? "lineMorningEnabled" : "lineEveningEnabled";
  const users = await prisma.user.findMany({
    where: { notificationPreference: { is: { OR: [{ [emailField]: true }, { [lineField]: true }] } } },
    select: {
      id: true,
      email: true,
      name: true,
      nickname: true,
      notificationPreference: { select: { morningEnabled: true, eveningEnabled: true, lineMorningEnabled: true, lineEveningEnabled: true } },
      lineConnection: { select: { lineUserId: true } },
      studyPlans: {
        where: { date: { gte: start, lt: end } },
        select: { done: true, content: true, subject: true, textbook: { select: { name: true } } },
      },
      studyLogs: { where: { date: { gte: start, lt: end } }, select: { minutes: true } },
    },
  });

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
        const delivery = await prisma.notificationDelivery.create({
          data: { userId: user.id, date: start, slot, channel },
          select: { id: true },
        });
        deliveryId = delivery.id;
      } catch (error) {
        if (isUniqueConstraintError(error)) {
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
        await prisma.notificationDelivery.delete({ where: { id: deliveryId } });
        console.error(`[daily-notification] ${slot}/${channel} delivery failed for user ${user.id}.`, error);
      }
    }
  }

  return { date, slot, eligible, sent, skipped, failed };
}
