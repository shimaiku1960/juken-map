import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

// studyPlans（サーバー状態）の型・取得・queryKey をここに集約する。
// フォーム/カレンダー/リストで共有し、鍵や取得処理の二重定義を防ぐ。

// 型は src/shared/dto/study.ts が正。ここは既存の import 元を変えずに済ませるための re-export。
// API 側はサービス（listStudyPlans）がこの形で組み立てて返す。
export type { Textbook, StudyPlan } from "@/shared/dto/study";
import type { StudyPlan } from "@/shared/dto/study";
import { api } from "@/web/lib/api-client";
import { dashboardKey } from "@/web/hooks/dashboard-key";
import type {
  CreateStudyPlansInput,
  UpdateStudyPlanInput,
} from "@/shared/validations/studyPlan";

// 予定を取りにいく期間（両端を含む "YYYY-MM-DD"）。
//
// 期間を必ず指定するのは、API が全期間・全件を返していた頃、平均75.5KBあって
// 1画面でいちばん大きな応答になっていたため。画面が本当に要る範囲だけを取る。
export type StudyPlanRange = { from: string; to: string };

// studyPlans キャッシュの唯一の住所。invalidate も含め全員がこれを参照する。
// 期間ごとに別のキャッシュになるが、無効化はこの前方一致でまとめて効く。
export const studyPlansKey = ["studyPlans"] as const;

// 予定のキャッシュの持ち方。実績と同じ考え方（useStudyLogs の STUDY_LOG_CACHE 参照）。
// 予定が変わるのは本人が作る・直す・完了するときだけで、そのとき invalidate が走る。
const STUDY_PLAN_CACHE = {
  staleTime: Infinity,
  gcTime: 60 * 60 * 1000,
} as const;

// サーバーから期間ぶんの studyPlans を取得する（useQuery の queryFn）
export async function fetchStudyPlans(range: StudyPlanRange): Promise<StudyPlan[]> {
  const params = new URLSearchParams({ from: range.from, to: range.to });
  return api.get<StudyPlan[]>(`/api/study-plans?${params}`, {
    fallbackMessage: "学習予定の取得に失敗しました",
  });
}

/**
 * 期間ぶんの予定のクエリ設定。useQuery と prefetchQuery の両方から使う。
 * 先読みと購読で queryKey がずれると別のキャッシュになってしまうので、1か所にまとめる。
 */
export function studyPlansQueryOptions(range: StudyPlanRange) {
  return {
    queryKey: [...studyPlansKey, "list", range.from, range.to],
    queryFn: () => fetchStudyPlans(range),
    ...STUDY_PLAN_CACHE,
  };
}

/**
 * 期間ぶんの予定を購読するフック。
 *
 * `enabled: false` のあいだは取りに行かない。カレンダーが当月を表示するときに使う＝
 * 当月の予定はダッシュボードの応答に同梱されてキャッシュへ入るので、それを待たずに
 * 走らせると同じ月を二重に取ってしまう。
 */
export function useStudyPlans(
  range: StudyPlanRange,
  options?: { enabled?: boolean }
) {
  return useQuery({
    ...studyPlansQueryOptions(range),
    enabled: options?.enabled ?? true,
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
      await queryClient.invalidateQueries({ queryKey: dashboardKey });
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
      await queryClient.invalidateQueries({ queryKey: dashboardKey });
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
      await queryClient.invalidateQueries({ queryKey: dashboardKey });
    },
  });
}
