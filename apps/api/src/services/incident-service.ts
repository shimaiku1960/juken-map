import { execute, select } from "@/api/infra/db";
import type { UserRow } from "@/api/infra/tables";

// 乗っ取りが起きたときに使う操作（pnpm incident が使う。手順は docs/incident-response.md）。
//
// 管理画面の「停止」は、自分自身・ほかの管理者を止められない（apps/api-go の admin_users.go）。
// 管理者が乗っ取られたときや、管理画面に入れないときでも使えるよう、ここではその守りを置かない。
// そのぶん、画面からは呼べず、本番では EC2 の API のコンテナの中からしか実行できない。

type Target = Pick<UserRow, "id" | "email" | "role" | "bannedAt">;

async function findByEmail(email: string) {
  const [user] = await select<Target>(
    "SELECT id, email, role, bannedAt FROM `user` WHERE email = ?",
    [email]
  );
  return user;
}

// セッションの取り消し（認証基準 10 の C5）。3＝ある利用者の全端末は deleteSessions、4＝全員は revokeAllSessions。
async function deleteSessions(userId: string) {
  const result = await execute("DELETE FROM AuthSession WHERE userId = ?", [userId]);
  return result.affectedRows;
}

/** その人のログイン中のセッションを、作られた順に並べる（どこから入られたかを見るため）。 */
export async function listSessionsByEmail(email: string) {
  const user = await findByEmail(email);
  if (!user) return undefined;
  const sessions = await select<{
    createdAt: Date;
    expiresAt: Date;
    lastUsedAt: Date;
    ipAddress: string | null;
    userAgent: string | null;
    twoFactorVerified: boolean;
  }>(
    `SELECT createdAt, expiresAt, lastUsedAt, ipAddress, userAgent, mfaVerifiedAt IS NOT NULL AS twoFactorVerified
     FROM AuthSession WHERE userId = ? ORDER BY createdAt ASC`,
    [user.id]
  );
  return { user, sessions };
}

/** その人のセッションをすべて消す。止めはしないので、パスワードを知っていればまた入れる。 */
export async function revokeSessionsByEmail(email: string) {
  const user = await findByEmail(email);
  if (!user) return undefined;
  return { user, sessionsRemoved: await deleteSessions(user.id) };
}

/**
 * その人を止め、セッションをすべて消す。止めた人は次のログインで断られる（apps/api-go の auth_handlers.go）。
 * bannedAt を先に書くので、消している間に新しく入られても、そのログインは断られる。
 * 止め直しても最初に止めた日時を保つ（管理画面の停止と同じ）。
 */
export async function banByEmail(email: string) {
  const user = await findByEmail(email);
  if (!user) return undefined;
  const now = new Date();
  await execute(
    "UPDATE `user` SET bannedAt = COALESCE(bannedAt, ?), updatedAt = ? WHERE id = ?",
    [now, now, user.id]
  );
  return { user, sessionsRemoved: await deleteSessions(user.id) };
}

/** 止めたのを戻す。 */
export async function unbanByEmail(email: string) {
  const user = await findByEmail(email);
  if (!user) return undefined;
  await execute("UPDATE `user` SET bannedAt = NULL, updatedAt = ? WHERE id = ?", [
    new Date(),
    user.id,
  ]);
  return { user };
}

/**
 * 全員のセッションを消す（C5 の4）。ログインの不具合や、セッションを読める立場（DB）からの漏えいが
 * 疑われるときに使う。全員がログインし直しになる。
 */
export async function revokeAllSessions() {
  const result = await execute("DELETE FROM AuthSession");
  return result.affectedRows;
}

/** 管理者全員のセッションを消す。管理者のアカウントが1つでも乗っ取られたかもしれないときに使う。 */
export async function revokeAdminSessions() {
  const admins = await select<{ id: string; email: string | null }>(
    "SELECT id, email FROM `user` WHERE role = 'admin' ORDER BY email ASC"
  );
  let sessionsRemoved = 0;
  for (const admin of admins) sessionsRemoved += await deleteSessions(admin.id);
  return { admins, sessionsRemoved };
}

/**
 * 2段階認証を設定する前に戻し、セッションをすべて消す。認証アプリの秘密が漏れたかもしれないときや、
 * 本人が認証アプリも予備コードも無くしたときに使う。次に /admin を開くと設定し直しになる。
 */
export async function resetTwoFactorByEmail(email: string) {
  const user = await findByEmail(email);
  if (!user) return undefined;
  await execute("DELETE FROM AuthTotp WHERE userId = ?", [user.id]);
  await execute("DELETE FROM AuthBackupCode WHERE userId = ?", [user.id]);
  await execute("DELETE FROM AuthMfaChallenge WHERE userId = ?", [user.id]);
  await execute("UPDATE `user` SET twoFactorEnabled = false, updatedAt = ? WHERE id = ?", [
    new Date(),
    user.id,
  ]);
  return { user, sessionsRemoved: await deleteSessions(user.id) };
}
