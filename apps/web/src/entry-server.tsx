// Node の上で React を動かして HTML を作る入口。ブラウザには配らない。
// - SSG：ビルドの最後に scripts/prerender.mjs が呼び、ファイルに書き出す
// - SSR：apps/api/src/spa.ts がリクエストのたびに呼ぶ
import "@/web/lib/zod-jitless";
import { prerender } from "react-dom/static";
import { StaticRouter } from "react-router";
import { dehydrate, QueryClient, type QueryKey } from "@tanstack/react-query";
import Root from "@/web/Root";
import { blogDetailKey, type Blog } from "@/web/hooks/useBlog";
import { QUERY_STATE_ELEMENT_ID } from "@/web/lib/query-state";

// 誰が見ても同じ中身で、ビルドのたびに作り直せば足りるページだけを並べる。
// ログインで中身が変わるページ（トップ・ログイン後の画面）は入れない。
export const SSG_PATHS = ["/terms", "/privacy"];

const ROOT_MARKER = '<div id="root"></div>';

/** 描く前にキャッシュへ入れておくデータ。画面の useQuery は取りに行かず、これを使う。 */
export type InitialData = Array<[QueryKey, unknown]>;

/**
 * index.html（ひな形）の空の root に、url のページを描いた HTML を入れて返す。
 * initialData を渡したときは、同じデータを HTML に埋め込み、ブラウザへ引き継ぐ。
 */
export async function renderPage(template: string, url: string, initialData: InitialData = []) {
  if (template.split(ROOT_MARKER).length !== 2) {
    throw new Error(`ひな形に ${ROOT_MARKER} がちょうど1つ必要です`);
  }

  const queryClient = new QueryClient();
  for (const [key, data] of initialData) queryClient.setQueryData(key, data);

  // renderToString は Suspense の中身を待たず fallback を出すので、React.lazy のページが空になる。
  // prerender は Suspense がすべて解決するまで待ってから HTML を返す。
  const { prelude } = await prerender(
    <Root
      queryClient={queryClient}
      router={({ children }) => <StaticRouter location={url}>{children}</StaticRouter>}
    />,
    {
      // 既定（約12.8KB）より大きい Suspense の中身は、隠した要素とインライン script で
      // 後から差し込む形で出てくる（少しずつ届くストリーミング向けの作り）。規約の本文は
      // これを超えるうえ、本番の CSP はハッシュの無いインライン script を止めるので本文が
      // 隠れたままになる。ここでは1枚の HTML を丸ごと返すので、後回しにせず本文をその場に書く。
      progressiveChunkSize: Number.POSITIVE_INFINITY,
    }
  );
  const appHtml = await new Response(prelude).text();

  // サーバーで使ったデータをブラウザへ渡す。渡さないと、ブラウザは最初に「読み込み中」を描き、
  // サーバーの HTML（記事の本文）と食い違ってハイドレーションが失敗する。
  // type="application/json" は実行されないので CSP に止められない。本文に "</script>" が
  // 含まれていてもタグが閉じないよう、"<" を JSON の文字コード表記に置き換える。
  const stateTag = initialData.length
    ? `<script id="${QUERY_STATE_ELEMENT_ID}" type="application/json">${JSON.stringify(
        dehydrate(queryClient)
      ).replaceAll("<", "\\u003c")}</script>`
    : "";

  return template.replace(ROOT_MARKER, `<div id="root">${appHtml}</div>${stateTag}`);
}

/** 記事のページを描く（SSR。apps/api/src/spa.ts がリクエストのたびに呼ぶ）。 */
export function renderArticlePage(template: string, blog: Blog) {
  const pathname = `/articles/${blog.id}`;
  return renderPage(template, pathname, [[blogDetailKey(blog.id), blog]]);
}
