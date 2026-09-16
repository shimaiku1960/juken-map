import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { Textbook } from "@/web/hooks/useStudyPlans";
import type {
  CreateTextbookInput,
  UpdateTextbookProgressInput,
} from "@/shared/validations/textbook";
import { api } from "@/web/lib/api-client";

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
  return api.get<Textbook[]>("/api/textbooks", {
    fallbackMessage: "参考書の取得に失敗しました",
  });
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
  return api.get<TextbookMaster[]>("/api/textbook-masters", {
    fallbackMessage: "参考書マスターの取得に失敗しました",
  });
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
    mutationFn: ({
      id,
      data,
    }: {
      id: number;
      data: UpdateTextbookProgressInput;
    }): Promise<Textbook> =>
      api.patch<Textbook>(`/api/textbooks/${id}`, data, {
        fallbackMessage: "設定に失敗しました",
      }),
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

// 参考書の科目だけを更新するフック（逆算設定を持たない参考書でも科目を設定できる）。
// 科目は参考書に紐づくため、更新するとその参考書を使う予定・実績すべてに反映される。
export function useUpdateTextbookSubject() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      subject,
    }: {
      id: number;
      subject: string | null;
    }): Promise<Textbook> =>
      api.patch<Textbook>(
        `/api/textbooks/${id}`,
        { subject },
        { fallbackMessage: "科目の設定に失敗しました" }
      ),
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
    mutationFn: (data: CreateTextbookInput): Promise<Textbook> =>
      api.post<Textbook>("/api/textbooks", data, {
        fallbackMessage: "参考書の追加に失敗しました",
      }),
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
