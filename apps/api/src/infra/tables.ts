// テーブル1行の形。Prisma がスキーマから自動生成していた型の代わりに手で書く。
//
// 列を足しても DB からは何も教えてくれないので、ここを直し忘れても型エラーにならない。
// ORM を外すと、スキーマと型の一致を保つのは人間の責任になる。
// 列の定義は prisma/migrations の CREATE TABLE / ALTER TABLE が正。

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
