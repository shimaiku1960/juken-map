import { useQuery } from "@tanstack/react-query";

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
    queryFn: async (): Promise<NotificationPreferences> => {
      const res = await fetch("/api/notification-preferences");
      if (!res.ok) throw new Error("通知設定の取得に失敗しました");
      return res.json();
    },
  });
}

export const lineConnectionKey = ["line-connection"] as const;

export function useLineConnection() {
  return useQuery({
    queryKey: lineConnectionKey,
    queryFn: async (): Promise<{ connected: boolean }> => {
      const res = await fetch("/api/line/connection");
      if (!res.ok) throw new Error("LINE連携状態の取得に失敗しました");
      return res.json();
    },
  });
}
