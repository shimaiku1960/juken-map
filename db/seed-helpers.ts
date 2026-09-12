// seed スクリプトで共通に使う書き込み。アプリと同じ接続プール（apps/api/src/infra/db.ts）を
// 使うので、日時は UTC で保存され、真偽値の扱いもアプリと揃う。
//
// Prisma のときは各 seed が同じ「確認済みユーザー＋パスワード」の upsert を
// それぞれ書いていたので、ここに1つにまとめた。
import { generateId } from "better-auth";
import { hashPassword } from "better-auth/crypto";
import { execute, pool, select } from "../apps/api/src/infra/db";
import { ymdAfterDays } from "../src/shared/date";

export { execute, select };

// アプリの開発サーバーと違い、seed では流した SQL を表示しない（負荷試験用の seed は
// 標準出力を JSON として読まれる）。見たいときは SQL_LOG=on を付けて実行する。
process.env.SQL_LOG ??= "off";

/**
 * seed の main を実行し、最後に接続プールを閉じる。閉じないとプロセスが終わらない。
 */
export function runSeed(main: () => Promise<void>) {
  main()
    .catch((error) => {
      console.error(error);
      process.exitCode = 1;
    })
    .finally(() => pool.end());
}

/**
 * メール確認済みのユーザーを用意し、id を返す。既にあれば確認済みに戻す。
 *
 * user は id と email の2つが UNIQUE なので、INSERT ... ON DUPLICATE KEY UPDATE で書くと
 * どちらの重複でも発動する。ここでは email で探してから UPDATE か INSERT を選ぶ。
 */
export async function upsertVerifiedUser(
  profile: { email: string; name: string; nickname: string },
  options: {
    /** 既存ユーザーの名前とニックネームも profile の値に戻す */
    resetProfile?: boolean;
    /** 初回記録の印を消し、次の記録を「初回」として扱わせる */
    resetFirstStudyLog?: boolean;
  } = {}
) {
  const now = new Date();
  const [existing] = await select<{ id: string }>(
    "SELECT id FROM `user` WHERE email = ?",
    [profile.email]
  );

  if (existing) {
    const changes: [column: string, value: unknown][] = [
      ["emailVerified", true],
      ["updatedAt", now],
    ];
    if (options.resetProfile) {
      changes.push(["name", profile.name], ["nickname", profile.nickname]);
    }
    if (options.resetFirstStudyLog) changes.push(["firstStudyLogAt", null]);
    await execute(
      `UPDATE \`user\` SET ${changes.map(([column]) => `${column} = ?`).join(", ")} WHERE id = ?`,
      [...changes.map(([, value]) => value), existing.id]
    );
    return existing.id;
  }

  // id は Better Auth が登録時に作るのと同じ形式にする
  const id = generateId();
  await execute(
    `INSERT INTO \`user\` (id, email, name, nickname, emailVerified, createdAt, updatedAt)
     VALUES (?, ?, ?, ?, TRUE, ?, ?)`,
    [id, profile.email, profile.name, profile.nickname, now, now]
  );
  return id;
}

/**
 * メールアドレスとパスワードでログインできるようにする（Better Auth の credential アカウント）。
 * パスワードは Better Auth と同じ関数でハッシュにする。再実行時は上書きする。
 */
export async function setCredentialPassword(userId: string, password: string) {
  const passwordHash = await hashPassword(password);
  const now = new Date();
  const [existing] = await select<{ id: string }>(
    "SELECT id FROM account WHERE userId = ? AND providerId = 'credential' LIMIT 1",
    [userId]
  );
  if (existing) {
    await execute("UPDATE account SET password = ?, updatedAt = ? WHERE id = ?", [
      passwordHash,
      now,
      existing.id,
    ]);
    return;
  }
  await execute(
    `INSERT INTO account (id, userId, accountId, providerId, password, createdAt, updatedAt)
     VALUES (?, ?, ?, 'credential', ?, ?, ?)`,
    [generateId(), userId, userId, passwordHash, now, now]
  );
}

/** 大学名と学部名から学部の id を引く。無ければ、先に通常の seed を流すよう促す。 */
export async function findFacultyId(university: string, faculty: string) {
  const [row] = await select<{ id: number }>(
    `SELECT f.id FROM Faculty AS f
     JOIN University AS u ON u.id = f.universityId
     WHERE u.name = ? AND f.name = ?
     ORDER BY f.id ASC LIMIT 1`,
    [university, faculty]
  );
  if (!row) {
    throw new Error(`${university} ${faculty} が見つかりません。先に通常のseedを実行してください`);
  }
  return row.id;
}

/** 学習予定をまとめて入れる。日付は「今日から何日後か」で指定する（UTC の 0 時として保存）。 */
export async function insertStudyPlans(
  userId: string,
  plans: { offset: number; content: string; subject: string; done?: boolean }[]
) {
  if (plans.length === 0) return;
  const now = new Date();
  await execute(
    `INSERT INTO StudyPlan (userId, date, content, subject, done, createdAt, updatedAt)
     VALUES ${plans.map(() => "(?, ?, ?, ?, ?, ?, ?)").join(", ")}`,
    plans.flatMap((plan) => [
      userId,
      new Date(ymdAfterDays(plan.offset)),
      plan.content,
      plan.subject,
      plan.done ?? false,
      now,
      now,
    ])
  );
}

/** 学習実績をまとめて入れる。日付は「今日から何日後か」で指定する。 */
export async function insertStudyLogs(
  userId: string,
  logs: { offset: number; subject: string; minutes: number }[]
) {
  if (logs.length === 0) return;
  const now = new Date();
  await execute(
    `INSERT INTO StudyLog (userId, date, subject, minutes, createdAt, updatedAt)
     VALUES ${logs.map(() => "(?, ?, ?, ?, ?, ?)").join(", ")}`,
    logs.flatMap((log) => [
      userId,
      new Date(ymdAfterDays(log.offset)),
      log.subject,
      log.minutes,
      now,
      now,
    ])
  );
}
