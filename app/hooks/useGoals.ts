import { useQuery } from "@tanstack/react-query";

// goals（サーバー状態）の型・取得・queryKey をここに集約する。
// GoalList / FacultyList など複数コンポーネントで共有し、鍵や取得処理の
// 二重定義（タイポで共有が壊れる事故）を防ぐ。

export type Faculty = {
  id: number;
  name: string;
  examDate: Date;
  university: { name: string };
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
  const res = await fetch("/api/goals");
  if (!res.ok) throw new Error("目標の取得に失敗しました");
  return res.json();
}

// goals を購読するフック。SSR で取得済みの initialGoals があれば初期キャッシュに使う。
export function useGoals(initialGoals?: Goal[]) {
  return useQuery({
    queryKey: goalsKey,
    queryFn: fetchGoals,
    initialData: initialGoals,
  });
}
