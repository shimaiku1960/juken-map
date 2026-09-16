import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { PatchGoalInput } from "@/shared/validations/goal";
import { api } from "@/web/lib/api-client";

// goals（サーバー状態）の型・取得・queryKey をここに集約する。
// GoalList / FacultyList など複数コンポーネントで共有し、鍵や取得処理の
// 二重定義（タイポで共有が壊れる事故）を防ぐ。

export type Faculty = {
  id: number;
  name: string;
  examDate: Date;
  university: { name: string; type: string; prefecture: string };
  tags: { name: string }[];
};

export type GoalStatus = "candidate" | "decided";

export type Goal = {
  id: number;
  createdAt: Date;
  userId: string;
  isFirstChoice: boolean;
  note: string | null;
  status: GoalStatus;
  faculty: Faculty;
};

// goals キャッシュの唯一の住所。invalidate も含め全員がこれを参照する。
export const goalsKey = ["goals"] as const;

// サーバーから最新の goals を取得する（useQuery の queryFn）
export async function fetchGoals(): Promise<Goal[]> {
  return api.get<Goal[]>("/api/goals", {
    fallbackMessage: "目標の取得に失敗しました",
  });
}

// goals を購読するフック。SSR で取得済みの initialGoals があれば初期キャッシュに使う。
export function useGoals(initialGoals?: Goal[]) {
  return useQuery({
    queryKey: goalsKey,
    queryFn: fetchGoals,
    initialData: initialGoals,
  });
}

// 志望校を削除するフック（成功したら一覧を再取得）。
// トーストなど画面への知らせは、呼び出し側が mutate の onSuccess / onError で出す。
export function useDeleteGoal() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) =>
      api.del<void>(`/api/goals/${id}`, {
        fallbackMessage: "削除に失敗しました",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
    },
  });
}

// 志望校を部分更新するフック（第一志望の設定/解除、候補→受験校の確定など）。
// isPending は呼び出しごとに別なので、用途ごとにこのフックを呼び分ける。
export function useUpdateGoal() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: PatchGoalInput }) =>
      api.patch<void>(`/api/goals/${id}`, data, {
        fallbackMessage: "更新に失敗しました",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
    },
  });
}
