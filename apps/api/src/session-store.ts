import { createHash } from "node:crypto";
import { select } from "@/api/infra/db";

// ログインのセッションを読む（JUK-115）。ログインそのもの（発行・取り消し）は Go が行い
// （apps/api-go の auth_*.go）、Node は Cookie のトークンから DB の AuthSession を引くだけ。
// Node に残るログインの判定は /line/settings の振り分けだけ（routes/line.ts）。
//
// Cookie で運ぶのは 256 ビットの乱数のトークンで、DB にはその SHA-256 だけがある。
// 期限（上限・管理者の使わないときの期限）は Go と同じ条件で見る。最後に使った時刻は書き換えない。

/** セッションの Cookie の名前。Go の sessionCookieName と同じ。 */
export const SESSION_COOKIE = "__Host-jm_session";

export type Session = {
  user: { id: string; email: string; role: string; bannedAt: Date | null };
  session: { id: string; twoFactorVerified: boolean };
};

/** Cookie ヘッダーからセッションを引く。無い・形が違う・期限切れなら null。 */
export async function findSession(cookieHeader: string | undefined): Promise<Session | null> {
  const token = readCookie(cookieHeader, SESSION_COOKIE);
  // 256 ビットを = の無い base64url にすると 43 文字。形が違えば DB を引かない。
  if (!token || !/^[A-Za-z0-9_-]{43}$/.test(token)) return null;
  const tokenHash = createHash("sha256").update(token).digest();
  const now = new Date();
  const [row] = await select<{
    id: string;
    userId: string;
    email: string | null;
    role: string;
    bannedAt: Date | null;
    twoFactorVerified: number;
  }>(
    `SELECT s.id, s.userId, u.email, u.role, u.bannedAt, s.mfaVerifiedAt IS NOT NULL AS twoFactorVerified
     FROM AuthSession AS s JOIN \`user\` AS u ON u.id = s.userId
     WHERE s.tokenHash = ? AND s.expiresAt > ?
       AND (s.idleTimeoutSeconds IS NULL OR s.lastUsedAt > DATE_SUB(?, INTERVAL s.idleTimeoutSeconds SECOND))`,
    [tokenHash, now, now]
  );
  if (!row) return null;
  return {
    user: { id: row.userId, email: row.email ?? "", role: row.role, bannedAt: row.bannedAt },
    session: { id: row.id, twoFactorVerified: Boolean(row.twoFactorVerified) },
  };
}

function readCookie(header: string | undefined, name: string) {
  for (const part of header?.split(";") ?? []) {
    const [key, ...rest] = part.trim().split("=");
    if (key === name) return rest.join("=");
  }
  return undefined;
}
