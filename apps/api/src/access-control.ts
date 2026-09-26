import type { FastifyInstance, FastifyRequest, RouteOptions } from "fastify";
import { denyDemoWrite, getSession, requireAdmin, requireSession, type Session } from "./context.ts";

// 全ルートに「誰が呼んでよいか（入口の種類）」を1つずつ持たせ、認証・管理者・デモの拒否を
// ここで一括して行う。ハンドラごとに requireSession を書く方式だと、書き忘れたルートは
// 誰でも呼べてしまい、テストも CI も気づけない。
//
// 種類は dev-standards の脅威プロファイル（targets/06_threat-profile.md）の入口 E1〜E8 に
// 対応する。E2（認証）は Better Auth が server.ts で横取りする /api/auth/* で、Fastify の
// ルートとしては登録されない。E9・E10 は HTTP の入口ではない。

export const ACCESS_ENTRY = {
  /** E1 公開の読み取り。誰でも呼べる。 */
  public: "E1",
  /** E3 利用者の API。ログイン必須、デモは書き込み不可。持ち主の確認はハンドラが行う。 */
  user: "E3",
  /** E4 管理者の API。ログイン＋role=admin。 */
  admin: "E4",
  /** E5 外から呼ばれる機械の入口。署名はハンドラが確かめる（LINE Webhook）。 */
  webhook: "E5",
  /** E6 自分のジョブの入口。トークンはハンドラが確かめる（cron・sim）。 */
  job: "E6",
  /** E7 ログイン不要の書き込み（CSP 違反レポート）。書き込めても害が無い設計にする。 */
  "anonymous-write": "E7",
  /** E8 外部との連携（OAuth）。画面遷移なので、未ログインはハンドラがログインへ送る。デモは不可。 */
  oauth: "E8",
} as const;

export type Access = keyof typeof ACCESS_ENTRY;

declare module "fastify" {
  interface FastifyContextConfig {
    access?: Access;
  }
  interface FastifyRequest {
    /** access が user・admin・oauth のルートで、下のフックが入れる。 */
    session: Session | null;
  }
}

/** GET と HEAD 以外は書き込みとして扱い、デモを断る。 */
function isWrite(request: FastifyRequest) {
  return request.method !== "GET" && request.method !== "HEAD";
}

/**
 * 入口の種類に応じた拒否を、ハンドラより前（ボディの解析よりも前）に行う。
 * access の無いルートがあれば起動（ready）で失敗させる。
 */
export function registerAccessControl(app: FastifyInstance) {
  app.decorateRequest("session", null);

  // 登録の時点ではなく ready で確かめるのは、@fastify/static のように access を
  // 渡せないプラグインのルートへ、子のスコープの onRoute で後から付けるため（spa.ts）。
  // 子の onRoute は親の onRoute より後に走る。
  const routes: RouteOptions[] = [];
  app.addHook("onRoute", (route) => {
    routes.push(route);
  });
  app.addHook("onReady", async () => {
    const missing = routes
      .filter((route) => !route.config?.access)
      .map((route) => `${[route.method].flat().join(",")} ${route.url}`);
    if (missing.length > 0) {
      throw new Error(`入口の種類（config.access）が無いルートがあります: ${missing.join(" / ")}`);
    }
  });

  app.addHook("onRequest", async (request, reply) => {
    // 存在しないパス（404）と Better Auth の /api/auth/* は access を持たない。
    switch (request.routeOptions.config.access) {
      case "user": {
        const session = await requireSession(request, reply);
        if (!session) return reply;
        if (isWrite(request) && denyDemoWrite(session, reply)) return reply;
        request.session = session;
        return;
      }
      case "admin": {
        const session = await requireAdmin(request, reply);
        if (!session) return reply;
        request.session = session;
        return;
      }
      case "oauth": {
        // 未ログインでも止めない（ハンドラが戻り先つきでログイン画面へ送る）。
        const session = await getSession(request);
        if (session && denyDemoWrite(session, reply)) return reply;
        request.session = session;
        return;
      }
      default:
        return;
    }
  });
}

/**
 * access が user・admin のハンドラでセッションを取り出す。フックが通したあとなので必ずある。
 * 無ければ access の付け間違いなので、500 にして気づけるようにする。
 */
export function currentSession(request: FastifyRequest): Session {
  if (!request.session) {
    // request.url ではなくルートの型を出す（クエリにトークンが載る入口があるため）。
    throw new Error(
      `${request.method} ${request.routeOptions.url} はセッションを要しない入口として登録されています`
    );
  }
  return request.session;
}
