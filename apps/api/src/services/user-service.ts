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
      // eslint-disable-next-line no-restricted-syntax -- USER_COLUMNS は固定の列名の並び（定数）。id は ? で渡す
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

/**
 * 管理者の一覧（pnpm admin:grant --list が使う）。2段階認証を有効にしたか、パスワードで
 * ログインできるか（無ければ管理画面に入れない）も並べ、誰が管理者なのかを把握できるようにする。
 */
export function listAdmins() {
  return measured("user.listAdmins", () =>
    select<{ email: string | null; twoFactorEnabled: boolean; hasPassword: boolean }>(
      `SELECT u.email,
              EXISTS (SELECT 1 FROM AuthTotp AS t WHERE t.userId = u.id AND t.enabledAt IS NOT NULL) AS twoFactorEnabled,
              EXISTS (SELECT 1 FROM AuthPassword AS p WHERE p.userId = u.id) AS hasPassword
       FROM \`user\` AS u
       WHERE u.role = 'admin'
       ORDER BY u.email ASC`
    )
  );
}

/**
 * メールアドレスでユーザーを探して role を付け替える（pnpm admin:grant が使う）。
 * メール確認前のユーザーには admin を付けない。他人のアドレスで登録されただけの
 * アカウントを、確認前に管理者にしてしまわないため。
 *
 * 付け替えたら、その人のセッションをすべて消す（認証基準 10 の C4：権限の変更のたびに作り直す）。
 * セッションの期限は role で決まる（一般30日・管理者24時間、apps/api-go の auth_session.go）ので、
 * ログインし直して新しい期限のセッションにしてもらう。
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
    const removed = await execute("DELETE FROM AuthSession WHERE userId = ?", [user.id]);
    return { result: "updated" as const, previous: user.role, sessionsRemoved: removed.affectedRows };
  });
}
