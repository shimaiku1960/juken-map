import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type {
  CompleteStudyPlanInput,
  CreateStudyLogInput,
} from "@/shared/validations/studyLog";
import { studyPlansKey } from "@/web/hooks/useStudyPlans";
import { trackEvent } from "@/web/lib/analytics";

// studyLogs（勉強の「実績」＝サーバー状態）の型・取得・queryKey をここに集約する。
// 型は src/shared/dto/study.ts が正（re-export）。理由は useStudyPlans.ts のコメント参照。
export type { DailyStudyMinutes, StudyLog } from "@/shared/dto/study";
import type { DailyStudyMinutes, StudyLog } from "@/shared/dto/study";
import { api } from "@/web/lib/api-client";

// 実績の記録には入口が3つある（手入力・タイマー・予定の完了）が、GA4 へは
// 同じ形で送る。イベント名の出し分けを2か所に書かないよう、ここにまとめる。
type StudyLogRecordMethod = "manual" | "timer" | "plan";

function trackStudyLogCreated(
  isFirstStudyLog: boolean,
  recordMethod: StudyLogRecordMethod
) {
  trackEvent(
    isFirstStudyLog ? "first_study_log_created" : "study_log_created",
    { record_method: recordMethod }
  );
}

// 実績を取りにいく期間（両端を含む "YYYY-MM-DD"）。
//
// 期間を必ず指定するのは、API が全期間・全件を返していた頃、使い込んだ利用者で
// 1件の応答が610KBになっていたため。画面が本当に要る範囲だけを取る。
export type StudyLogRange = { from: string; to?: string };

// studyLogs キャッシュの唯一の住所。invalidate も含め全員がこれを参照する。
// 期間ごとに別のキャッシュになるが、無効化はこの前方一致でまとめて効く。
export const studyLogsKey = ["studyLogs"] as const;

function rangeQuery(range: StudyLogRange): string {
  const params = new URLSearchParams({ from: range.from });
  if (range.to !== undefined) params.set("to", range.to);
  return params.toString();
}

// サーバーから期間ぶんの studyLogs を取得する（useQuery の queryFn）
export async function fetchStudyLogs(range: StudyLogRange): Promise<StudyLog[]> {
  return api.get<StudyLog[]>(`/api/study-logs?${rangeQuery(range)}`, {
    fallbackMessage: "学習実績の取得に失敗しました",
  });
}

// 実績のキャッシュの持ち方。全クエリ共通の既定（staleTime 30秒）より長く持つ。
//
// 過ぎた日の記録は、本人が編集しない限り変わらない。編集・削除・新規記録はどれも
// studyLogsKey を invalidate するので、そのときは staleTime に関係なく取り直される。
// つまり「古いまま見えてしまう」経路が無く、期限で切る理由が無い。
// カレンダーの月送りを往復しても通信が起きないのは、この2つの設定のおかげ。
const STUDY_LOG_CACHE = {
  staleTime: Infinity,
  gcTime: 60 * 60 * 1000, // 画面から外れた月も1時間は捨てない
} as const;

/**
 * 期間ぶんの明細のクエリ設定。useQuery と prefetchQuery の両方から使う。
 * 先読みと購読で queryKey がずれると別のキャッシュになってしまうので、1か所にまとめる。
 */
export function studyLogsQueryOptions(range: StudyLogRange) {
  return {
    queryKey: [...studyLogsKey, "list", range.from, range.to ?? null],
    queryFn: () => fetchStudyLogs(range),
    ...STUDY_LOG_CACHE,
  };
}

// 期間ぶんの実績の明細を購読するフック。
export function useStudyLogs(range: StudyLogRange) {
  return useQuery(studyLogsQueryOptions(range));
}

// 日ごとの合計学習時間だけを購読するフック。連続記録日数のように、
// 明細は要らないが長い期間が要る集計に使う。
export function useDailyStudyMinutes(range: StudyLogRange) {
  return useQuery({
    queryKey: [...studyLogsKey, "daily", range.from, range.to ?? null],
    queryFn: () =>
      api.get<DailyStudyMinutes[]>(`/api/study-logs/daily?${rangeQuery(range)}`, {
        fallbackMessage: "学習実績の取得に失敗しました",
      }),
    ...STUDY_LOG_CACHE,
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
      trackStudyLogCreated(created.isFirstStudyLog, "manual");
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

export async function saveStudySession(input: SaveStudySessionInput) {
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

  // 応答に isFirstStudyLog が無い場合もあるので、ここで真偽に寄せる。
  return {
    isFirstStudyLog: result?.isFirstStudyLog === true,
    recordMethod: input.planId == null ? ("timer" as const) : ("plan" as const),
  };
}

export function useSaveStudySession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: saveStudySession,
    onSuccess: async ({ isFirstStudyLog, recordMethod }) => {
      trackStudyLogCreated(isFirstStudyLog, recordMethod);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: studyLogsKey }),
        queryClient.invalidateQueries({ queryKey: studyPlansKey }),
      ]);
    },
  });
}
