// フロントエンドとバックエンドの間で受け渡す形（DTO）の型定義。
//
// ここには型だけを置く。この形へ組み立てるのは apps/api のサービス
// （listStudyPlans / listStudyLogs）で、戻り値の型がそのままこの型になっている。
//
// 日付が Date ではなく ISO 文字列なのは、画面へは JSON で届き、JSON には Date 型がないため。
//
// 画面側の hooks（useStudyPlans / useStudyLogs）はこの型を re-export している。
// 画面と API の両方がこのファイルだけを見ればよい状態にしてある。

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
  textbook: Textbook | null; // JOIN で取得（表示用。名前だけ使う）
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  createdAt: string;
  updatedAt: string;
};

// ダッシュボードの初回表示ぶん。1画面を1リクエストで賄うためにまとめた形。
// logs は「月初と7日前の早い方 〜 月末」で、画面側が用途ごとに絞って使う。
export type StudyDashboard = {
  month: string; // "YYYY-MM"（サーバーが決めた対象月）
  from: string; // "YYYY-MM-DD"
  to: string; // "YYYY-MM-DD"
  logs: StudyLog[];
  dailyMinutes: DailyStudyMinutes[];
};

// 日別の合計学習時間。ヒートマップの連続記録日数のように「その日に何分やったか」
// しか要らない画面のために、明細ではなくこの形で返す（1年ぶんでも数十KBに収まる）。
export type DailyStudyMinutes = {
  date: string; // ISO 文字列（StudyLog.date と同じ形）
  minutes: number;
};

export type StudyLog = {
  id: number;
  userId: string;
  date: string; // ISO 文字列（JSON 経由で来るため）
  minutes: number;
  subject: string | null;
  textbookId: number | null;
  textbook: Textbook | null; // JOIN で取得（表示用。名前だけ使う）
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  memo: string | null;
  studyPlanId: number | null;
  createdAt: string;
  updatedAt: string;
};
