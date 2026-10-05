import { useEffect, useState } from "react";

// メールや LINE のリンク（?token=…・?linkToken=…）で開く画面のための、トークンの受け取り
// （認証基準 10 の D3）。
//
// URL に載ったトークンは、ブラウザの履歴・アクセスログ・外部の解析ツールに残る。読み込んだらすぐ
// URL から消し（history.replaceState）、この画面からほかのサイトへ移ったときに Referer で
// 送られないよう no-referrer にする（サーバーも同じ画面の HTML に Referrer-Policy: no-referrer を付ける）。
export function useTokenFromLink(param = "token") {
  const [token] = useState(() =>
    typeof window === "undefined" ? null : new URLSearchParams(window.location.search).get(param)
  );

  useEffect(() => {
    const meta = document.createElement("meta");
    meta.name = "referrer";
    meta.content = "no-referrer";
    document.head.appendChild(meta);

    const url = new URL(window.location.href);
    if (url.searchParams.has(param)) {
      url.searchParams.delete(param);
      window.history.replaceState(window.history.state, "", `${url.pathname}${url.search}${url.hash}`);
    }
    return () => meta.remove();
  }, [param]);

  return token;
}
