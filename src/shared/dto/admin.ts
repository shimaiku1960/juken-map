// 管理者ページ（/admin）で API と画面が受け渡す形。組み立ては apps/api/src/services/admin-service.ts。
// 日時は JSON で届くので ISO 文字列。

/**
 * 利用者の種別。合成ユーザーが実ユーザーの数字に混ざらないように分けて数える。
 *   real  実際の利用者
 *   sim   本番のシミュレーション（sim/）が作る合成ユーザー
 *   seed  手元の負荷検証用 seed（db/seed-synthetic.ts）が作る合成ユーザー
 *   demo  面接官向けのデモアカウント
 */
export const USER_KINDS = ["real", "sim", "seed", "demo"] as const;
export type UserKind = (typeof USER_KINDS)[number];

export type KindStats = {
  kind: UserKind;
  total: number;
  verified: number;
  newLast7Days: number;
  activeLast7Days: number;
  activeLast30Days: number;
};

export type AdminOverview = {
  kinds: KindStats[];
  /** 実ユーザーの日別新規登録（Asia/Tokyo の日付、直近30日。登録0の日は含まない）。 */
  realSignupsByDay: { date: string; count: number }[];
};

export type AdminUser = {
  id: string;
  email: string | null;
  nickname: string | null;
  name: string | null;
  kind: UserKind;
  role: "user" | "admin";
  emailVerified: boolean;
  createdAt: string;
  /** 認証方法（account.providerId）。"credential" はメール＋パスワード。 */
  providers: string[];
  lastLoginAt: string | null;
  studyLogCount: number;
  lastStudyLogAt: string | null;
};

export type AdminUserList = {
  users: AdminUser[];
  total: number;
  page: number;
  pageSize: number;
};
