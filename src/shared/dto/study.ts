// フロントエンドとバックエンドの間で受け渡す形（DTO）の型定義。
//
// ここには型だけを置き、Prisma からの変換関数は src/backend/dto/study-mapper.ts にある。
// shared は外部依存を持たない層なので、Prisma の型を参照しないための分割である。
//
// 日付が Date ではなく ISO 文字列なのは、同じデータが2経路で画面に届くため。
//   1. Server Component が Prisma を直接呼び、props で渡す  → Date のまま
//   2. ブラウザが fetch("/api/...") で受け取る               → JSON なので文字列
// JSON には Date 型がないので、型を1つに揃えるには文字列側へ寄せるしかない。
//
// frontend の app/hooks/ はこの型を re-export しており、backend の study-mapper が
// この形へ変換する。両側がこのファイルだけを見ればよい状態にしてある。

export type Textbook = {
  id: number;
  masterId: number | null;
  name: string;
  totalAmount: number | null;
  rangeUnit: string | null;
  targetDate: string | null;
  subject: string | null;
};

export type StudyPlan = {
  id: number;
  userId: string;
  date: string; // ISO 文字列（JSON 経由で来るため）
  content: string | null;
  subject: string | null;
  done: boolean;
  studyLogId: number | null;
  textbookId: number | null;
  textbook: Textbook | null; // include で取得（表示用。名前だけ使う）
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  createdAt: string;
  updatedAt: string;
};

export type StudyLog = {
  id: number;
  userId: string;
  date: string; // ISO 文字列（JSON 経由で来るため）
  minutes: number;
  subject: string | null;
  textbookId: number | null;
  textbook: Textbook | null; // include で取得（表示用。名前だけ使う）
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  memo: string | null;
  studyPlanId: number | null;
  createdAt: string;
  updatedAt: string;
};
