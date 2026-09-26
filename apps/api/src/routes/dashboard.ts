import type { FastifyInstance } from "fastify";
import { getDashboard } from "@/api/services/dashboard-service";
import { currentSession } from "../access-control.ts";

export function registerDashboardRoutes(app: FastifyInstance) {
  // ダッシュボードの初回表示ぶんを1回で返す。実績・予定・連続記録日数を別々に取ると
  // 1画面で何リクエストにもなり、そのたびにセッション照会が走る
  // （2026-09-23の限界点試験で、それが効いていると分かった）。
  //
  // パラメータは受け取らない。期間はサーバーが決める＝画面・シミュレーション・
  // 負荷試験で期間の計算が散らばらないようにするため。
  app.get("/api/dashboard", { config: { access: "user" } }, async (request) => {
    const session = currentSession(request);

    return getDashboard(session.user.id);
  });
}
