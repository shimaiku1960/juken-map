import { useMutation } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";

// LINE のトークから来た linkToken を使って連携を始める。
// 成功すると LINE 側へ遷移するので、サーバー状態のキャッシュは触らない
// （この画面はそのまま離脱する）。
export function useStartLineAccountLink() {
  return useMutation({
    mutationFn: (linkToken: string) =>
      api.post<{ redirectUrl: string }>(
        "/api/line/account-link",
        { linkToken },
        { fallbackMessage: "連携を開始できませんでした" }
      ),
  });
}
