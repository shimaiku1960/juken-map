// フロントエンドとバックエンドの間で受け渡す形（DTO）の型定義。
//
// ここには型だけを置く。この形へ組み立てるのは apps/api のサービス
// （listStudyPlans / listStudyLogs）で、戻り値の型がそのままこの型になっている。
//
// 日付が Date ではなく ISO 文字列なのは、画面へは JSON で届き、JSON には Date 型がないため。
//
// 画面側の hooks（useStudyPlans / useStudyLogs）はこの型を re-export している。
// 画面と API の両方がこのファイルだけを見ればよい状態にしてある。
//
// 予定と実績の一覧には userId・createdAt・updatedAt を入れない。画面はどれも使わず、
// 自分のデータしか返らないので userId は全行同じ値になる。この3つで実績の一覧の
// 約3割のバイト数を占めていた（2026-09-25、JUK-49）。

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
};

// 両端を含む期間（"YYYY-MM-DD"）。サーバーが何を返したかを画面へ伝えるのに使う。
export type DateRange = { from: string; to: string };

// ダッシュボードの初回表示ぶん。1画面を1リクエストで賄うためにまとめた形。
// 期間はサーバーが決める（画面・シミュレーション・負荷試験で計算が散らばらないように）。
// 実績は過去だけなので当月＋直近7日、予定は未来にもあるので当月＋今週。
// どちらも画面側が用途ごとに絞って使う。
export type Dashboard = {
  month: string; // "YYYY-MM"（今日の月）
  logRange: DateRange;
  logs: StudyLog[];
  planRange: DateRange;
  plans: StudyPlan[];
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
};
