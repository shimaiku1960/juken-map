import prisma from "@/lib/prisma";
import { getResend } from "@/lib/resend";
import {
  buildDailyNotification,
  tokyoDateRange,
  type NotificationSlot,
} from "@/lib/dailyNotification";

const FROM = "受験マップ <noreply@juken-map.com>";

function isUniqueConstraintError(error: unknown) {
  return (
    typeof error === "object" &&
    error !== null &&
    "code" in error &&
    error.code === "P2002"
  );
}

export async function sendDailyNotifications(
  slot: NotificationSlot,
  now = new Date()
) {
  const { date, start, end } = tokyoDateRange(now);
  const enabledField =
    slot === "morning" ? "morningEnabled" : "eveningEnabled";
  const users = await prisma.user.findMany({
    where: {
      email: { not: null },
      notificationPreference: { is: { [enabledField]: true } },
    },
    select: {
      id: true,
      email: true,
      name: true,
      nickname: true,
      studyPlans: {
        where: { date: { gte: start, lt: end } },
        select: {
          done: true,
          content: true,
          subject: true,
          textbook: { select: { name: true } },
        },
      },
      studyLogs: {
        where: { date: { gte: start, lt: end } },
        select: { minutes: true },
      },
    },
  });

  let sent = 0;
  let skipped = 0;
  let failed = 0;

  for (const user of users) {
    let deliveryId: number | null = null;
    try {
      const delivery = await prisma.notificationDelivery.create({
        data: { userId: user.id, date: start, slot },
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
      const message = buildDailyNotification({
        slot,
        nickname: user.nickname ?? user.name ?? "ユーザー",
        plans: user.studyPlans,
        logs: user.studyLogs,
      });
      const { error } = await getResend().emails.send({
        from: FROM,
        to: user.email!,
        ...message,
      });
      if (error) throw new Error(error.message);
      sent += 1;
    } catch (error) {
      failed += 1;
      await prisma.notificationDelivery.delete({ where: { id: deliveryId } });
      console.error(
        `[daily-notification] ${slot} delivery failed for user ${user.id}.`,
        error
      );
    }
  }

  return { date, slot, eligible: users.length, sent, skipped, failed };
}
