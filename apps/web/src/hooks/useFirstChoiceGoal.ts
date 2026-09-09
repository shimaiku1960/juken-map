import { useQuery } from "@tanstack/react-query";

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
    queryFn: async (): Promise<FirstChoiceGoal> => {
      const res = await fetch("/api/goals/first-choice");
      if (!res.ok) throw new Error("第一志望の取得に失敗しました");
      return res.json();
    },
  });
}
