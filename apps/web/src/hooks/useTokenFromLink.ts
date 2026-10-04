import { useEffect, useState } from "react";

// メールのリンク（?token=…）で開く画面のための、トークンの受け取り（認証基準 10 の D3）。
//
// URL に載ったトークンは、ブラウザの履歴・アクセスログ・外部の解析ツールに残る。読み込んだらすぐ
// URL から消し（history.replaceState）、この画面からほかのサイトへ移ったときに Referer で
// 送られないよう no-referrer にする（サーバーも同じ画面の HTML に Referrer-Policy: no-referrer を付ける）。
export function useTokenFromLink() {
  const [token] = useState(() =>
    typeof window === "undefined" ? null : new URLSearchParams(window.location.search).get("token")
  );

  useEffect(() => {
    const meta = document.createElement("meta");
    meta.name = "referrer";
    meta.content = "no-referrer";
    document.head.appendChild(meta);

    const url = new URL(window.location.href);
    if (url.searchParams.has("token")) {
      url.searchParams.delete("token");
      window.history.replaceState(window.history.state, "", `${url.pathname}${url.search}${url.hash}`);
    }
    return () => meta.remove();
  }, []);

  return token;
}
