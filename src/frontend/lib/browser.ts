export function isLineInAppBrowser(userAgent: string) {
  return /(?:\bLine\/|\bLIFF\b)/i.test(userAgent);
}

export function safeCallbackURL(search: string, fallback: string) {
  const requested = new URLSearchParams(search).get("callbackURL");
  return requested?.startsWith("/") && !requested.startsWith("//")
    ? requested
    : fallback;
}
