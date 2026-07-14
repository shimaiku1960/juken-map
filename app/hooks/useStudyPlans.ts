import { useQuery } from "@tanstack/react-query";

// studyPlans（サーバー状態）の型・取得・queryKey をここに集約する。
// フォーム/カレンダー/リストで共有し、鍵や取得処理の二重定義を防ぐ。

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

// studyPlans キャッシュの唯一の住所。invalidate も含め全員がこれを参照する。
export const studyPlansKey = ["studyPlans"] as const;

// サーバーから最新の studyPlans を取得する（useQuery の queryFn）
export async function fetchStudyPlans(): Promise<StudyPlan[]> {
  const res = await fetch("/api/study-plans");
  if (!res.ok) throw new Error("学習予定の取得に失敗しました");
  return res.json();
}

// studyPlans を購読するフック。SSR で取得済みの initialPlans があれば初期キャッシュに使う。
export function useStudyPlans(initialPlans?: StudyPlan[]) {
  return useQuery({
    queryKey: studyPlansKey,
    queryFn: fetchStudyPlans,
    initialData: initialPlans,
  });
}
