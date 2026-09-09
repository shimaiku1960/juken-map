import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { CreateStudyLogInput } from "@/shared/validations/studyLog";
import { trackEvent } from "@/frontend/lib/analytics";

// studyLogs（勉強の「実績」＝サーバー状態）の型・取得・queryKey をここに集約する。
// 型は src/shared/dto/study.ts が正（re-export）。理由は useStudyPlans.ts のコメント参照。
export type { StudyLog } from "@/shared/dto/study";
import type { StudyLog } from "@/shared/dto/study";

// studyLogs キャッシュの唯一の住所。invalidate も含め全員がこれを参照する。
export const studyLogsKey = ["studyLogs"] as const;

// サーバーから最新の studyLogs を取得する（useQuery の queryFn）
export async function fetchStudyLogs(): Promise<StudyLog[]> {
  const res = await fetch("/api/study-logs");
  if (!res.ok) throw new Error("学習実績の取得に失敗しました");
  return res.json();
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
    mutationFn: async (data: CreateStudyLogInput) => {
      const res = await fetch("/api/study-logs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(
          typeof err.error === "string" ? err.error : "記録に失敗しました"
        );
      }
      return res.json() as Promise<StudyLog & { isFirstStudyLog: boolean }>;
    },
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
    mutationFn: async ({
      id,
      data,
    }: {
      id: number;
      data: CreateStudyLogInput;
    }) => {
      const res = await fetch(`/api/study-logs/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(
          typeof err.error === "string" ? err.error : "編集に失敗しました"
        );
      }
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: studyLogsKey });
    },
  });
}

// 実績を削除するフック（成功したら一覧を再取得）
export function useDeleteStudyLog() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: number) => {
      const res = await fetch(`/api/study-logs/${id}`, { method: "DELETE" });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(
          typeof err.error === "string" ? err.error : "削除に失敗しました"
        );
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: studyLogsKey });
    },
  });
}
