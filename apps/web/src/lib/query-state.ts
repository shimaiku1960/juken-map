import { hydrate, type QueryClient } from "@tanstack/react-query";

// サーバー（SSR・SSG）が HTML を作るときに使ったデータを、ブラウザへ渡すための要素の id。
// 書き込むのは entry-server.tsx、読むのは main.tsx。
export const QUERY_STATE_ELEMENT_ID = "query-state";

/**
 * HTML に埋め込まれたデータを、描く前にキャッシュへ入れる。
 * こうすると画面の useQuery は最初からデータを持ち、サーバーと同じものを描ける。
 */
export function restoreQueryState(queryClient: QueryClient) {
  const element = document.getElementById(QUERY_STATE_ELEMENT_ID);
  if (!element?.textContent) return;
  hydrate(queryClient, JSON.parse(element.textContent));
}
