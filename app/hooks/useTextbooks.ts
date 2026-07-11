import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { Textbook } from "@/app/hooks/useStudyPlans";
import type {
  CreateTextbookInput,
  UpdateTextbookProgressInput,
} from "@/lib/validations/textbook";

// textbooks（サーバー状態）の型・取得・queryKey をここに集約する。
export const textbooksKey = ["textbooks"] as const;
export const textbookMastersKey = ["textbookMasters"] as const;

export type TextbookMasterMetric = {
  id: number;
  unit: string;
  totalAmount: number;
  isDefault: boolean;
};

export type TextbookMaster = {
  id: number;
  name: string;
  publisher: string | null;
  edition: string | null;
  isbn: string;
  metrics: TextbookMasterMetric[];
};

// サーバーから自分の参考書一覧を取得する
async function fetchTextbooks(): Promise<Textbook[]> {
  const res = await fetch("/api/textbooks");
  if (!res.ok) throw new Error("参考書の取得に失敗しました");
  return res.json();
}

// 参考書一覧を購読するフック
export function useTextbooks(initialData?: Textbook[]) {
  return useQuery({
    queryKey: textbooksKey,
    queryFn: fetchTextbooks,
    initialData,
  });
}

async function fetchTextbookMasters(): Promise<TextbookMaster[]> {
  const res = await fetch("/api/textbook-masters");
  if (!res.ok) throw new Error("参考書マスターの取得に失敗しました");
  return res.json();
}

export function useTextbookMasters() {
  return useQuery({
    queryKey: textbookMastersKey,
    queryFn: fetchTextbookMasters,
  });
}

export function useUpdateTextbookProgress() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      id,
      data,
    }: {
      id: number;
      data: UpdateTextbookProgressInput;
    }): Promise<Textbook> => {
      const res = await fetch(`/api/textbooks/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const error = await res.json();
        throw new Error(
          typeof error.error === "string" ? error.error : "設定に失敗しました"
        );
      }
      return res.json();
    },
    onSuccess: (updated) => {
      queryClient.setQueryData<Textbook[]>(textbooksKey, (current = []) =>
        current.map((textbook) =>
          textbook.id === updated.id ? updated : textbook
        )
      );
      queryClient.invalidateQueries({ queryKey: textbooksKey });
    },
  });
}

// 参考書を新規追加するフック（成功したら一覧を再取得して選択肢を最新化）
export function useCreateTextbook() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CreateTextbookInput): Promise<Textbook> => {
      const res = await fetch("/api/textbooks", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "参考書の追加に失敗しました");
      }
      return res.json();
    },
    onSuccess: (created) => {
      queryClient.setQueryData<Textbook[]>(textbooksKey, (current = []) =>
        current.some((textbook) => textbook.id === created.id)
          ? current
          : [...current, created]
      );
      queryClient.invalidateQueries({ queryKey: textbooksKey });
    },
  });
}
