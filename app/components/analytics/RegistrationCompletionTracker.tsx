"use client";

import { useEffect } from "react";
import { trackEvent } from "@/lib/analytics";

export default function RegistrationCompletionTracker() {
  useEffect(() => {
    void fetch("/api/analytics/registration", { method: "POST" })
      .then((response) => (response.ok ? response.json() : null))
      .then((result: { shouldTrack?: boolean; method?: string } | null) => {
        if (result?.shouldTrack && result.method) {
          trackEvent("sign_up", { method: result.method });
        }
      })
      .catch(() => {
        // 計測失敗は登録後の利用を妨げない。
      });
  }, []);

  return null;
}
