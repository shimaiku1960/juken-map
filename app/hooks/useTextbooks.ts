import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { Textbook } from "@/app/hooks/useStudyPlans";

// textbooks（サーバー状態）の型・取得・queryKey をここに集約する。
export const textbooksKey = ["textbooks"] as const;

// サーバーから自分の参考書一覧を取得する
async function fetchTextbooks(): Promise<Textbook[]> {
  const res = await fetch("/api/textbooks");
  if (!res.ok) throw new Error("参考書の取得に失敗しました");
  return res.json();
}

// 参考書一覧を購読するフック
export function useTextbooks() {
  return useQuery({
    queryKey: textbooksKey,
    queryFn: fetchTextbooks,
  });
}

// 参考書を新規追加するフック（成功したら一覧を再取得して選択肢を最新化）
export function useCreateTextbook() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (name: string): Promise<Textbook> => {
      const res = await fetch("/api/textbooks", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "参考書の追加に失敗しました");
      }
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: textbooksKey });
    },
  });
}
