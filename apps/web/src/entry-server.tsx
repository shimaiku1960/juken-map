// ビルド時に Node で動かし、SSG するページの HTML を作る入口（scripts/prerender.mjs から呼ぶ）。
// ブラウザには配らない。
import "@/web/lib/zod-jitless";
import { prerender } from "react-dom/static";
import { StaticRouter } from "react-router";
import { QueryClient } from "@tanstack/react-query";
import Root from "@/web/Root";

// 誰が見ても同じ中身で、ビルドのたびに作り直せば足りるページだけを並べる。
// ログインで中身が変わるページ（トップ・ログイン後の画面）は入れない。
export const SSG_PATHS = ["/terms", "/privacy"];

export async function render(url: string) {
  // renderToString は Suspense の中身を待たず fallback を出すので、React.lazy のページが空になる。
  // prerender は Suspense がすべて解決するまで待ってから HTML を返す（SSG 用の API）。
  const { prelude } = await prerender(
    <Root
      queryClient={new QueryClient()}
      router={({ children }) => <StaticRouter location={url}>{children}</StaticRouter>}
    />,
    {
      // 既定（約12.8KB）より大きい Suspense の中身は、隠した要素とインライン script で
      // 後から差し込む形で出てくる（少しずつ届くストリーミング向けの作り）。規約の本文は
      // これを超えるうえ、本番の CSP はハッシュの無いインライン script を止めるので本文が
      // 隠れたままになる。SSG は1枚の HTML を丸ごと返すので、後回しにせず本文をその場に書く。
      progressiveChunkSize: Number.POSITIVE_INFINITY,
    }
  );
  return new Response(prelude).text();
}
