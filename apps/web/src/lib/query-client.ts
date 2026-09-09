import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // SPA では初回表示のデータもここから取る（Server Component の直呼びが無くなるため）。
      // 画面遷移のたびに取り直さないよう、少しだけ新鮮とみなす。
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});
