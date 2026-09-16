import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";
import type { NotificationPreferenceInput } from "@/shared/validations/notification";

export type NotificationPreferences = {
  emailMorningEnabled: boolean;
  emailEveningEnabled: boolean;
  lineMorningEnabled: boolean;
  lineEveningEnabled: boolean;
};

export const notificationPreferencesKey = ["notification-preferences"] as const;

export function useNotificationPreferences() {
  return useQuery({
    queryKey: notificationPreferencesKey,
    queryFn: () =>
      api.get<NotificationPreferences>("/api/notification-preferences", {
        fallbackMessage: "通知設定の取得に失敗しました",
      }),
  });
}

export const lineConnectionKey = ["line-connection"] as const;

export function useLineConnection() {
  return useQuery({
    queryKey: lineConnectionKey,
    queryFn: () =>
      api.get<{ connected: boolean }>("/api/line/connection", {
        fallbackMessage: "LINE連携状態の取得に失敗しました",
      }),
  });
}

// 通知設定を保存するフック。
// 保存前は取得だけがキャッシュ経由で、更新は画面側の useState を手で書き換えて
// いたため、同じ値の置き場所が2つあった。保存結果をキャッシュへ書いて
// invalidate する形に統一し、画面はキャッシュだけを見ればよくする。
export function useSaveNotificationPreferences() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: NotificationPreferenceInput) =>
      api.put<NotificationPreferences>("/api/notification-preferences", data, {
        fallbackMessage: "保存に失敗しました",
      }),
    onSuccess: (saved) => {
      // 応答が保存後の値そのものなので、まずキャッシュへ入れて画面を確定させ、
      // そのうえで再取得する（先に invalidate だけだと再取得が終わるまで
      // 古い値が残り、保存ボタンが一瞬押せる状態に戻ってしまう）。
      queryClient.setQueryData(notificationPreferencesKey, saved);
      queryClient.invalidateQueries({ queryKey: notificationPreferencesKey });
    },
  });
}

// LINE連携を解除するフック。
// サーバー側（disconnectLine）が連携の削除と同時に LINE 通知の設定も落とすので、
// 通知設定のキャッシュも一緒に無効化する。
export function useDisconnectLine() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () =>
      api.del<{ connected: boolean }>("/api/line/connection", {
        fallbackMessage: "LINE連携を解除できませんでした",
      }),
    onSuccess: async () => {
      queryClient.setQueryData(lineConnectionKey, { connected: false });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: lineConnectionKey }),
        queryClient.invalidateQueries({ queryKey: notificationPreferencesKey }),
      ]);
    },
  });
}
