import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

// studyPlans（サーバー状態）の型・取得・queryKey をここに集約する。
// フォーム/カレンダー/リストで共有し、鍵や取得処理の二重定義を防ぐ。

// 型は src/shared/dto/study.ts が正。ここは既存の import 元を変えずに済ませるための re-export。
// API 側はサービス（listStudyPlans）がこの形で組み立てて返す。
export type { Textbook, StudyPlan } from "@/shared/dto/study";
import type { StudyPlan } from "@/shared/dto/study";
import { api } from "@/web/lib/api-client";
import type {
  CreateStudyPlansInput,
  UpdateStudyPlanInput,
} from "@/shared/validations/studyPlan";

// studyPlans キャッシュの唯一の住所。invalidate も含め全員がこれを参照する。
export const studyPlansKey = ["studyPlans"] as const;

// サーバーから最新の studyPlans を取得する（useQuery の queryFn）
export async function fetchStudyPlans(): Promise<StudyPlan[]> {
  return api.get<StudyPlan[]>("/api/study-plans", {
    fallbackMessage: "学習予定の取得に失敗しました",
  });
}

// studyPlans を購読するフック。SSR で取得済みの initialPlans があれば初期キャッシュに使う。
export function useStudyPlans(initialPlans?: StudyPlan[]) {
  return useQuery({
    queryKey: studyPlansKey,
    queryFn: fetchStudyPlans,
    initialData: initialPlans,
  });
}

// 学習予定を追加するフック。
// 一覧の再取得まで待ってから呼び出し側の onSuccess が走るよう、onSuccess で
// invalidateQueries を await する（追加直後にカレンダーが古いままにならない）。
export function useCreateStudyPlan() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateStudyPlansInput) =>
      api.post<void>("/api/study-plans", data, {
        fallbackMessage: "予定を追加できませんでした",
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: studyPlansKey });
    },
  });
}

// 学習予定を更新するフック（完了の切り替え、内容の編集）。
export function useUpdateStudyPlan() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: UpdateStudyPlanInput }) =>
      api.patch<void>(`/api/study-plans/${id}`, data, {
        fallbackMessage: "予定の更新に失敗しました",
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: studyPlansKey });
    },
  });
}

// 学習予定を削除するフック。
export function useDeleteStudyPlan() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) =>
      api.del<void>(`/api/study-plans/${id}`, {
        fallbackMessage: "予定の削除に失敗しました",
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: studyPlansKey });
    },
  });
}
