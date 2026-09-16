import { api } from "@/web/lib/api-client";

type AnalyticsValue = string | number | boolean;

declare global {
  interface Window {
    gtag?: (...args: unknown[]) => void;
  }
}

export function trackEvent(
  eventName: string,
  parameters: Record<string, AnalyticsValue> = {}
) {
  if (typeof window === "undefined" || !window.gtag) return;
  window.gtag("event", eventName, parameters);
}

// 本登録の完了を GA4 の sign_up として1回だけ数える。
// 二重計測を避ける判定はサーバーが持っているので、まず問い合わせてから送る。
// 計測は撃ちっぱなしでよい（失敗しても登録後の利用を妨げない）ため、
// フックにはせず、呼び出し側は戻り値を待たない。
export function reportRegistrationCompletion() {
  void api
    .post<{ shouldTrack?: boolean; method?: string }>(
      "/api/analytics/registration"
    )
    .then((result) => {
      if (result?.shouldTrack && result.method) {
        trackEvent("sign_up", { method: result.method });
      }
    })
    .catch(() => {
      // 計測失敗は登録後の利用を妨げない。
    });
}
