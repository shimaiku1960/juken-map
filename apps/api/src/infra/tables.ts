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
};

export type UniversityRow = {
  id: number;
  name: string;
  prefecture: string;
  type: string;
  createdAt: Date;
};

export type FacultyRow = {
  id: number;
  name: string;
  examDate: Date;
  createdAt: Date;
  universityId: number;
};

// 学部とタグは多対多。Prisma が暗黙に作った中間テーブル _FacultyToTag
// （A = Faculty.id, B = Tag.id）で結ばれている。
export type TagRow = {
  id: number;
  name: string;
  createdAt: Date;
};

// 志望校。1ユーザー1学部1行（userId, facultyId が UNIQUE）。更新日時の列は無い。
export type FinalGoalRow = {
  id: number;
  createdAt: Date;
  userId: string;
  facultyId: number;
  isFirstChoice: boolean;
  note: string | null;
  // "candidate"（比較検討中）/ "decided"（受験校として確定）
  status: string;
};

// LINE 連携。userId と lineUserId がそれぞれ UNIQUE（1アカウント1LINE、1LINE1アカウント）。
export type LineConnectionRow = {
  id: number;
  userId: string;
  lineUserId: string;
  linkedAt: Date;
  createdAt: Date;
  updatedAt: Date;
};

// LINE 公式アカウントからの連携（Account Linking）で使う、使い捨ての合言葉。
export type LineLinkNonceRow = {
  nonce: string;
  userId: string;
  expiresAt: Date;
  createdAt: Date;
};

// LINE Login の進行中の試行。state が主キーで、使い捨て。
export type LineOAuthAttemptRow = {
  state: string;
  userId: string;
  nonce: string;
  codeVerifier: string;
  redirectUri: string;
  expiresAt: Date;
  createdAt: Date;
};

// 参考書マスター（ISBN 単位の既製の参考書）と、その総量の候補（ページ数・問題数など）。
export type TextbookMasterRow = {
  id: number;
  name: string;
  publisher: string | null;
  edition: string | null;
  isbn: string;
  createdAt: Date;
  updatedAt: Date;
};

export type TextbookMasterMetricRow = {
  id: number;
  masterId: number;
  unit: string;
  totalAmount: number;
  isDefault: boolean;
  createdAt: Date;
  updatedAt: Date;
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
