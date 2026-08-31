"use client";

import { useSyncExternalStore } from "react";
import { isLineInAppBrowser, safeCallbackURL } from "@/lib/browser";

const subscribeToBrowserNavigation = (onStoreChange: () => void) => {
  window.addEventListener("pageshow", onStoreChange);
  window.addEventListener("popstate", onStoreChange);
  queueMicrotask(onStoreChange);
  return () => {
    window.removeEventListener("pageshow", onStoreChange);
    window.removeEventListener("popstate", onStoreChange);
  };
};

function useBrowserSnapshot<T>(getSnapshot: () => T, serverSnapshot: T) {
  return useSyncExternalStore(subscribeToBrowserNavigation, getSnapshot, () => serverSnapshot);
}

export function useIsLineInAppBrowser() {
  return useBrowserSnapshot(
    () => isLineInAppBrowser(window.navigator.userAgent),
    false
  );
}

export function useSafeCallbackURL(fallback: string) {
  const search = useBrowserSnapshot(() => window.location.search, "");
  return safeCallbackURL(search, fallback);
}
