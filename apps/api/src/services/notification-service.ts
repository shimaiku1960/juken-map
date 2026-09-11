import { execute, select } from "@/api/infra/db";
import { measured } from "@/api/observability/measured";

type NotificationPreference = {
  morningEnabled: boolean;
  eveningEnabled: boolean;
  lineMorningEnabled: boolean;
  lineEveningEnabled: boolean;
};

const PREFERENCE_COLUMNS =
  "morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled";

// プロフィール画面と通知設定 API の両方から使う。
export function findNotificationPreference(userId: string) {
  return measured("notificationPreference.find", async () => {
    const [row] = await select<NotificationPreference>(
      `SELECT ${PREFERENCE_COLUMNS} FROM NotificationPreference WHERE userId = ?`,
      [userId]
    );
    return row ?? null;
  });
}

// LINE 連携の有無だけを判定する。存在確認が目的なので id しか引かない。
export function findLineConnection(userId: string) {
  return measured("lineConnection.find", async () => {
    const [row] = await select<{ id: number }>(
      "SELECT id FROM LineConnection WHERE userId = ?",
      [userId]
    );
    return row ?? null;
  });
}

/** 通知設定を保存する。無ければ作り、あれば置き換える。 */
export function saveNotificationPreference(
  userId: string,
  data: NotificationPreference
) {
  return measured("notificationPreference.save", async () => {
    const now = new Date();
    // 「無ければ INSERT、あれば UPDATE」を1文で行う（userId に UNIQUE 制約がある）。
    // INSERT しようとして userId が重複したら、そのまま ON DUPLICATE KEY UPDATE に切り替わる。
    // new は「INSERT しようとした行」の別名。createdAt は更新しない。
    //
    // Prisma の upsert は MySQL では「SELECT で有無を見てから INSERT か UPDATE」の
    // 2本に分けていた。その間に別のリクエストが割り込めるが、1文ならその隙間が無い。
    await execute(
      `INSERT INTO NotificationPreference
         (userId, morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled,
          createdAt, updatedAt)
       VALUES (?, ?, ?, ?, ?, ?, ?) AS new
       ON DUPLICATE KEY UPDATE
         morningEnabled = new.morningEnabled,
         eveningEnabled = new.eveningEnabled,
         lineMorningEnabled = new.lineMorningEnabled,
         lineEveningEnabled = new.lineEveningEnabled,
         updatedAt = new.updatedAt`,
      [
        userId,
        data.morningEnabled,
        data.eveningEnabled,
        data.lineMorningEnabled,
        data.lineEveningEnabled,
        now,
        now,
      ]
    );
    const [saved] = await select<NotificationPreference>(
      `SELECT ${PREFERENCE_COLUMNS} FROM NotificationPreference WHERE userId = ?`,
      [userId]
    );
    return saved;
  });
}
