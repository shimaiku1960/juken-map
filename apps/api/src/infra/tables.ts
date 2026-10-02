// テーブル1行の形。Prisma がスキーマから自動生成していた型の代わりに手で書く。
//
// 列を足しても DB からは何も教えてくれないので、ここを直し忘れても型エラーにならない。
// ORM を外すと、スキーマと型の一致を保つのは人間の責任になる。
// 列の定義は db/migrations の CREATE TABLE / ALTER TABLE が正。

// Better Auth も同じテーブルを読み書きする。列は Better Auth の分も含めて全部書く。
export type UserRow = {
  id: string;
  name: string | null;
  email: string | null;
  image: string | null;
  nickname: string | null;
  createdAt: Date;
  updatedAt: Date;
  emailVerified: boolean;
  firstStudyLogAt: Date | null;
  analyticsSignUpTrackedAt: Date | null;
  // シミュレーションの合成ユーザーだけが持つ。実ユーザーは全部 NULL。
  simSeq: number | null;
  simCohort: string | null;
  simDormantFrom: Date | null;
  simLastActedOn: Date | null;
  // 管理者ページの権限（db/migrations/20260919090000_add_user_role）。
  role: UserRole;
  // 管理者に停止された日時。NULL なら停止していない（db/migrations/20260920090000_add_user_banned）。
  bannedAt: Date | null;
};

export type UserRole = "user" | "admin";

export type UniversityRow = {
  id: number;
  name: string;
  prefecture: string;
  type: string;
  createdAt: Date;
};

export type TextbookRow = {
  id: number;
  userId: string;
  masterId: number | null;
  name: string;
  totalAmount: number | null;
  rangeUnit: string | null;
  targetDate: Date | null;
  subject: string | null;
  createdAt: Date;
  updatedAt: Date;
};

export type StudyPlanRow = {
  id: number;
  userId: string;
  date: Date;
  content: string | null;
  subject: string | null;
  done: boolean;
  textbookId: number | null;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  createdAt: Date;
  updatedAt: Date;
};

export type StudyLogRow = {
  id: number;
  userId: string;
  date: Date;
  subject: string | null;
  minutes: number;
  textbookId: number | null;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  memo: string | null;
  studyPlanId: number | null;
  createdAt: Date;
  updatedAt: Date;
};
