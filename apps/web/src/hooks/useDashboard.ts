import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ymdLocal } from "@/shared/date";
import type { Dashboard } from "@/shared/dto/study";
import { api } from "@/web/lib/api-client";
import { monthRange, studyLogsQueryOptions } from "@/web/hooks/useStudyLogs";
import { studyPlansQueryOptions } from "@/web/hooks/useStudyPlans";

export type { Dashboard } from "@/shared/dto/study";

export const dashboardKey = ["dashboard"] as const;

/**
 * ダッシュボードの初回表示ぶんを1回で取るフック。
 *
 * 併せて、返ってきた実績と予定のうち対象月ぶんを「その月のキャッシュ」に書き込む。
 * カレンダーは月ごとのキャッシュを見る作りのままなので、こうしておくと当月を表示する
 * あいだは取得が起きない（staleTime が Infinity のため）。カレンダーの側にコードは
 * 要らず、月を送ったときだけ通常どおり取りに行く。
 *
 * ⚠️ 書き込みが届く前にカレンダーが走ると同じ月を二重に取る。カレンダーの取得は
 * このフックが終わるまで止めてある（StudyHeatmap の ready）。
 */
export function useDashboard() {
  const queryClient = useQueryClient();

  return useQuery({
    queryKey: dashboardKey,
    queryFn: async () => {
      const data = await api.get<Dashboard>("/api/dashboard", {
        fallbackMessage: "ダッシュボードの取得に失敗しました",
      });
      const range = monthRange(data.month);
      const inMonth = (date: string) => {
        const ymd = ymdLocal(date);
        return ymd >= range.from && ymd <= range.to;
      };
      queryClient.setQueryData(
        studyLogsQueryOptions(range).queryKey,
        data.logs.filter((log) => inMonth(log.date))
      );
      queryClient.setQueryData(
        studyPlansQueryOptions(range).queryKey,
        data.plans.filter((plan) => inMonth(plan.date))
      );
      return data;
    },
    staleTime: Infinity,
    gcTime: 60 * 60 * 1000,
  });
}
