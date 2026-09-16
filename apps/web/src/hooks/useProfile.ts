import { useMutation } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";
import type { ProfileInput } from "@/shared/validations/profile";

// プロフィール（ニックネーム）の更新。
// 表示名は Better Auth のセッションが持っていて TanStack Query のキャッシュには
// 無いため、ここでは invalidateQueries を呼ばない。更新後の反映は呼び出し側が
// useSession の refetch で行う（authClient.getSession() は購読ストアを更新せず、
// 表示が古いままになる）。
export function useUpdateProfile() {
  return useMutation({
    mutationFn: (data: ProfileInput) =>
      api.put<void>("/api/profile", data, {
        fallbackMessage: "更新に失敗しました",
      }),
  });
}
