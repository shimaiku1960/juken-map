import { useQuery } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";

// トップの「受験まであと何日」に使う第一志望。Next.js では Server Component が
// findFirstChoiceGoal を直接呼んでいたので API が無く、分離にあたり新設した。
export type FirstChoiceGoal = {
  id: number;
  faculty: {
    name: string;
    examDate: string;
    university: { name: string };
  };
} | null;

export const firstChoiceGoalKey = ["goals", "first-choice"] as const;

export function useFirstChoiceGoal(enabled: boolean) {
  return useQuery({
    queryKey: firstChoiceGoalKey,
    enabled,
    queryFn: () =>
      api.get<FirstChoiceGoal>("/api/goals/first-choice", {
        fallbackMessage: "第一志望の取得に失敗しました",
      }),
  });
}
