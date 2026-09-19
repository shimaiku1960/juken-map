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

// ---- マスター編集（/admin/masters） ----

export type AdminUniversity = {
  id: number;
  name: string;
  prefecture: string;
  type: string;
  facultyCount: number;
  /** 配下の学部を志望校にしている件数。1件でもあれば大学は削除できない。 */
  goalCount: number;
};

export type AdminUniversityList = {
  universities: AdminUniversity[];
  total: number;
  page: number;
  pageSize: number;
};

export type AdminTag = { id: number; name: string };

export type AdminFaculty = {
  id: number;
  name: string;
  /** YYYY-MM-DD（UTC の0時で保存している日付部分）。 */
  examDate: string;
  tags: AdminTag[];
  /** この学部を志望校にしている件数。1件でもあれば削除できない。 */
  goalCount: number;
};

export type AdminUniversityDetail = {
  university: AdminUniversity;
  faculties: AdminFaculty[];
};

export type AdminTextbookMaster = {
  id: number;
  name: string;
  publisher: string | null;
  edition: string | null;
  isbn: string;
  metrics: { unit: string; totalAmount: number; isDefault: boolean }[];
  /** この参考書から登録された利用者の参考書の数。1冊でもあれば削除できない。 */
  textbookCount: number;
};
