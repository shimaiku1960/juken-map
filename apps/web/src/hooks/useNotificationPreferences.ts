import { useQuery } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";

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
