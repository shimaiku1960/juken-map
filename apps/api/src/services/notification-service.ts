import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

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

/** 通知設定を保存する。無ければ作り、あれば置き換える。 */
export function saveNotificationPreference(
  userId: string,
  data: {
    morningEnabled: boolean;
    eveningEnabled: boolean;
    lineMorningEnabled: boolean;
    lineEveningEnabled: boolean;
  }
) {
  return measured("notificationPreference.save", () =>
    prisma.notificationPreference.upsert({
      where: { userId },
      create: { userId, ...data },
      update: data,
      select: {
        morningEnabled: true,
        eveningEnabled: true,
        lineMorningEnabled: true,
        lineEveningEnabled: true,
      },
    })
  );
}
