import { useQuery } from "@tanstack/react-query";

// studyPlans（サーバー状態）の型・取得・queryKey をここに集約する。
// フォーム/カレンダー/リストで共有し、鍵や取得処理の二重定義を防ぐ。

// 型は src/shared/dto/study.ts が正。ここは既存の import 元を変えずに済ませるための re-export。
// API 側はサービス（listStudyPlans）がこの形で組み立てて返す。
export type { Textbook, StudyPlan } from "@/shared/dto/study";
import type { StudyPlan } from "@/shared/dto/study";

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
