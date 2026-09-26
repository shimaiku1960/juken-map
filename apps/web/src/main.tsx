// Zod より先に読み込む（理由はファイル内）。
import "@/web/lib/zod-jitless";
import { createRoot, hydrateRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { queryClient } from "@/web/lib/query-client";
import { startFaro } from "@/web/lib/faro";
import Root from "@/web/Root";
import "@/web/index.css";

const container = document.getElementById("root")!;
const root = <Root queryClient={queryClient} router={BrowserRouter} />;

// SSG したページ（scripts/prerender.mjs）は、HTML の時点で本文が入っている。
// それを捨てて描き直さず、今ある DOM にイベントや状態を結びつける（ハイドレーション）。
// それ以外のページは中身が空なので、今までどおりブラウザで一から描く。
if (container.hasChildNodes()) {
  hydrateRoot(container, root);
} else {
  createRoot(container).render(root);
}

void startFaro();
