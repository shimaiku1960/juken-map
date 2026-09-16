import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type {
  CompleteStudyPlanInput,
  CreateStudyLogInput,
} from "@/shared/validations/studyLog";
import { studyPlansKey } from "@/web/hooks/useStudyPlans";
import { trackEvent } from "@/web/lib/analytics";

// studyLogs（勉強の「実績」＝サーバー状態）の型・取得・queryKey をここに集約する。
// 型は src/shared/dto/study.ts が正（re-export）。理由は useStudyPlans.ts のコメント参照。
export type { StudyLog } from "@/shared/dto/study";
import type { StudyLog } from "@/shared/dto/study";
import { api } from "@/web/lib/api-client";

// studyLogs キャッシュの唯一の住所。invalidate も含め全員がこれを参照する。
export const studyLogsKey = ["studyLogs"] as const;

// サーバーから最新の studyLogs を取得する（useQuery の queryFn）
export async function fetchStudyLogs(): Promise<StudyLog[]> {
  return api.get<StudyLog[]>("/api/study-logs", {
    fallbackMessage: "学習実績の取得に失敗しました",
  });
}

// studyLogs を購読するフック。SSR で取得済みの initialLogs があれば初期キャッシュに使う。
export function useStudyLogs(initialLogs?: StudyLog[]) {
  return useQuery({
    queryKey: studyLogsKey,
    queryFn: fetchStudyLogs,
    initialData: initialLogs,
  });
}

// 実績を新規記録するフック（成功したら一覧を再取得して集計を最新化）
export function useCreateStudyLog() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateStudyLogInput) =>
      api.post<StudyLog & { isFirstStudyLog: boolean }>(
        "/api/study-logs",
        data,
        { fallbackMessage: "記録に失敗しました" }
      ),
    onSuccess: (created) => {
      trackEvent(
        created.isFirstStudyLog
          ? "first_study_log_created"
          : "study_log_created",
        { record_method: "manual" }
      );
      queryClient.invalidateQueries({ queryKey: studyLogsKey });
    },
  });
}

// 実績を編集するフック（成功したらカレンダーと集計を再取得）
export function useUpdateStudyLog() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      data,
    }: {
      id: number;
      data: CreateStudyLogInput;
    }) =>
      api.patch<StudyLog>(`/api/study-logs/${id}`, data, {
        fallbackMessage: "編集に失敗しました",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: studyLogsKey });
    },
  });
}

// 実績を削除するフック（成功したら一覧を再取得）
export function useDeleteStudyLog() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) =>
      api.del<void>(`/api/study-logs/${id}`, {
        fallbackMessage: "削除に失敗しました",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: studyLogsKey });
    },
  });
}

// タイマーで計った学習を保存するフック。
//
// 予定から始めた学習は「予定を完了して実績も記録する」別の API を呼ぶため、
// 呼び出し先が planId の有無で変わる。この出し分けは保存という1つの操作の
// 内側の話なので、画面ではなくここに置く。
//
// 実績が増えると予定側の「実績記録済み」も変わるので、どちらの経路でも
// studyLogs と studyPlans の両方を無効化する。
export type SaveStudySessionInput =
  | { planId: null; data: CreateStudyLogInput }
  | { planId: number; data: CompleteStudyPlanInput };

export function useSaveStudySession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (input: SaveStudySessionInput) => {
      const result =
        input.planId == null
          ? await api.post<{ isFirstStudyLog?: boolean }>(
              "/api/study-logs",
              input.data,
              { fallbackMessage: "実績を保存できませんでした" }
            )
          : await api.post<{ isFirstStudyLog?: boolean }>(
              `/api/study-plans/${input.planId}/complete`,
              input.data,
              { fallbackMessage: "実績を保存できませんでした" }
            );

      return {
        isFirstStudyLog: result?.isFirstStudyLog === true,
        recordMethod: input.planId == null ? "timer" : "plan",
      } as const;
    },
    onSuccess: async ({ isFirstStudyLog, recordMethod }) => {
      trackEvent(
        isFirstStudyLog ? "first_study_log_created" : "study_log_created",
        { record_method: recordMethod }
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: studyLogsKey }),
        queryClient.invalidateQueries({ queryKey: studyPlansKey }),
      ]);
    },
  });
}
