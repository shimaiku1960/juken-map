import { StrictMode } from "react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { Toaster } from "@/web/components/ui/sonner";
import App from "@/web/App";

// ブラウザ（main.tsx）とビルド時の HTML 生成（entry-server.tsx）で同じ木を描くための共通部分。
// 両者で木が1か所でも違うと、ハイドレーションで「サーバーの HTML と合わない」エラーになる。
// ルーターだけは違う（ブラウザは BrowserRouter、ビルド時は URL を渡す StaticRouter）ので外から包む。
export default function Root({
  queryClient,
  router: Router,
}: {
  queryClient: QueryClient;
  router: React.ComponentType<{ children: React.ReactNode }>;
}) {
  return (
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <Router>
          <Toaster />
          <App />
        </Router>
      </QueryClientProvider>
    </StrictMode>
  );
}
