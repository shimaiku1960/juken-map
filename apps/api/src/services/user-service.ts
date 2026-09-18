import { execute, select } from "@/api/infra/db";
import type { UserRole, UserRow } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";

const USER_COLUMNS = `
  id, name, email, image, nickname, createdAt, updatedAt,
  emailVerified, firstStudyLogAt, analyticsSignUpTrackedAt
`;

/** ニックネームなどプロフィールを更新する。 */
export function updateProfile(userId: string, data: { nickname: string }) {
  return measured("user.updateProfile", async () => {
    await execute(
      "UPDATE `user` SET nickname = ?, updatedAt = ? WHERE id = ?",
      [data.nickname, new Date(), userId]
    );
    // 更新後の行を返す（Prisma の update と同じ）。MySQL の UPDATE は行を返さない。
    const [user] = await select<UserRow>(
      `SELECT ${USER_COLUMNS} FROM \`user\` WHERE id = ?`,
      [userId]
    );
    if (!user) throw new Error(`user ${userId} が見つかりません`);
    return user;
  });
}

/**
 * 新規登録の計測を「まだ送っていなければ」記録する。
 *
 * WHERE に analyticsSignUpTrackedAt IS NULL を入れて DB 側で判定させることで、
 * 同じユーザーが同時に2回開いても計測が二重に飛ばない。後から来た UPDATE は
 * 先の UPDATE の確定を待ち、そのときには条件に合わなくなっているので0行になる。
 * 戻り値 true は「今回が初回だった」の意味。
 */
export function markSignUpTracked(userId: string) {
  return measured("user.markSignUpTracked", async () => {
    const now = new Date();
    const result = await execute(
      `UPDATE \`user\` SET analyticsSignUpTrackedAt = ?, updatedAt = ?
       WHERE id = ? AND analyticsSignUpTrackedAt IS NULL`,
      [now, now, userId]
    );
    return result.affectedRows > 0;
  });
}

/** 登録に使われた認証方法を、最初に作られたアカウントから判定する。 */
export function findSignUpMethod(userId: string) {
  return measured("user.findSignUpMethod", async () => {
    // account は Better Auth が作るテーブル。メール登録なら providerId は "credential"。
    const [account] = await select<{ providerId: string }>(
      "SELECT providerId FROM account WHERE userId = ? ORDER BY createdAt ASC LIMIT 1",
      [userId]
    );
    return account?.providerId === "google" || account?.providerId === "github"
      ? account.providerId
      : "email";
  });
}

/**
 * メールアドレスでユーザーを探して role を付け替える（pnpm admin:grant が使う）。
 * メール確認前のユーザーには admin を付けない。他人のアドレスで登録されただけの
 * アカウントを、確認前に管理者にしてしまわないため。
 */
export function setUserRoleByEmail(email: string, role: UserRole) {
  return measured("user.setRoleByEmail", async () => {
    const [user] = await select<Pick<UserRow, "id" | "emailVerified" | "role">>(
      "SELECT id, emailVerified, role FROM `user` WHERE email = ?",
      [email]
    );
    if (!user) return { result: "not_found" as const };
    if (role === "admin" && !user.emailVerified) return { result: "unverified" as const };

    await execute("UPDATE `user` SET role = ?, updatedAt = ? WHERE id = ?", [
      role,
      new Date(),
      user.id,
    ]);
    return { result: "updated" as const, previous: user.role };
  });
}
