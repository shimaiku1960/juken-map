import type { FastifyInstance } from "fastify";
import { SITE_URL } from "@/shared/site";
import { getSession } from "../context.ts";

// LINE 連携の API（/api/line/*）は Go が受ける（JUK-79、Node からは JUK-84 で削除）。
// ここに残すのは、LINE のメッセージ本文が案内する /line/settings だけ。/api の外なので nginx は Node へ送る。

// 分離前は「リダイレクト先の画面」と「API」が同じオリジンだったので
// new URL(request.url).origin で足りていた。分離後は画面が別プロセスになるため、
// 戻り先はフロントのオリジンを明示する。本番は nginx で同一オリジンなので SITE_URL。
function webOrigin() {
  return process.env.WEB_ORIGIN ?? SITE_URL;
}

const NOTIFICATION_SETTINGS_PATH = "/profile#notification-settings";

export function registerLineRoutes(app: FastifyInstance) {
  // 画面を持たず、ログイン状態で行き先を変えるだけ。
  // Next.js では app/line/settings/route.ts が同じことをしていた。SPA 側のルートに
  // しないのは、描画が要らずクライアント判定だと一瞬ちらつくため。
  app.get("/line/settings", { config: { access: "public" } }, async (request, reply) => {
    const session = await getSession(request);
    const origin = webOrigin();

    if (session) {
      return reply.redirect(`${origin}${NOTIFICATION_SETTINGS_PATH}`);
    }

    const loginUrl = new URL("/login", origin);
    loginUrl.searchParams.set("callbackURL", NOTIFICATION_SETTINGS_PATH);
    return reply.redirect(loginUrl.toString());
  });
}
