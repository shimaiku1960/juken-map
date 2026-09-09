import prisma from "@/lib/prisma";
import { measured } from "@/lib/observability/measured";

// プロフィール画面（Server Component）と通知設定 API の両方から使う。
// 以前は両者に同じ findUnique が別々に書かれていた。
export function findNotificationPreference(userId: string) {
  return measured("notificationPreference.find", () =>
    prisma.notificationPreference.findUnique({
      where: { userId },
      select: {
        morningEnabled: true,
        eveningEnabled: true,
        lineMorningEnabled: true,
        lineEveningEnabled: true,
      },
    })
  );
}

// LINE 連携の有無だけを判定する。存在確認が目的なので id しか引かない。
export function findLineConnection(userId: string) {
  return measured("lineConnection.find", () =>
    prisma.lineConnection.findUnique({
      where: { userId },
      select: { id: true },
    })
  );
}
