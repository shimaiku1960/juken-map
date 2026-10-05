import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";
import { useTokenFromLink } from "@/web/hooks/useTokenFromLink";

// linkToken を同じタブの中で持ち回るための sessionStorage のキー。
const LINK_TOKEN_KEY = "juken-map:line-link-token";

function readStoredLinkToken(): string | null {
  try {
    return window.sessionStorage.getItem(LINK_TOKEN_KEY);
  } catch {
    return null;
  }
}

function writeStoredLinkToken(value: string | null) {
  try {
    if (value) window.sessionStorage.setItem(LINK_TOKEN_KEY, value);
    else window.sessionStorage.removeItem(LINK_TOKEN_KEY);
  } catch {
    // 預けられなくても、この画面の中では連携できる（ログインをはさむと、LINE で送り直してもらう）。
  }
}

// LINE のトークに届くリンク（/line/link?linkToken=…）の linkToken。
//
// URL に載ったままだと GA4・Faro へページの URL ごと送られるので、読み込んだらすぐ URL から消す
// （useTokenFromLink）。未ログインならログインをはさんでこの画面へ戻るが、戻り先の URL
// （callbackURL）にも載せたくないので、同じタブの sessionStorage に預けておく（JUK-124）。
export function useLineLinkToken() {
  const fromUrl = useTokenFromLink("linkToken");
  const [linkToken] = useState(() => fromUrl ?? readStoredLinkToken());

  useEffect(() => {
    if (fromUrl) writeStoredLinkToken(fromUrl);
  }, [fromUrl]);

  return linkToken;
}

// LINE のトークから来た linkToken を使って連携を始める。
// 成功すると LINE 側へ遷移するので、サーバー状態のキャッシュは触らない
// （この画面はそのまま離脱する）。linkToken は使い捨てなので、預けた分も消す。
export function useStartLineAccountLink() {
  return useMutation({
    mutationFn: (linkToken: string) =>
      api.post<{ redirectUrl: string }>(
        "/api/line/account-link",
        { linkToken },
        { fallbackMessage: "連携を開始できませんでした" }
      ),
    onSuccess: () => writeStoredLinkToken(null),
  });
}
