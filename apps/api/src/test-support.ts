import Fastify, {
  type FastifyInstance,
  type LightMyRequestResponse,
} from "fastify";
import { registerAccessControl } from "./access-control.ts";
import { BODY_LIMIT, registerErrorHandling } from "./error-handling.ts";
import { fastifyLoggingOptions, testLogLines } from "./observability/logger.ts";
import { runWithRequestContext } from "./observability/requestContext.ts";

/**
 * テスト用に、対象のルートだけを載せた Fastify を作る。
 *
 * buildServer() をそのまま使わないのは、全ルートを読み込むと microCMS や Resend の
 * クライアント生成が走り、テストに関係のない環境変数を要求するため。
 * JSON の解析とボディ上限・エラー応答の形は本番と同じにしておく（不正な JSON を 400 に
 * せず undefined を渡し、認証チェックを先に効かせる。理由は server.ts のコメント参照）。
 * ロガーも本番と同じものを渡す（テストでは画面に出さず testLogLines に溜まる）。
 */
export function buildTestApp(register: (app: FastifyInstance) => void) {
  const app = Fastify({ ...fastifyLoggingOptions, bodyLimit: BODY_LIMIT });

  // server.ts と同じく、リクエストごとの文脈を一番先に張る。
  // ここを省くと measured() や db.ts のログに reqId が付かず、テストが本番と別物になる。
  app.addHook("onRequest", (request, _reply, done) => {
    runWithRequestContext({ reqId: String(request.id) }, done);
  });

  // エラー応答の形（5xx の内部情報を隠す・reqId を返す）も本番と同じにする。
  registerErrorHandling(app);

  app.addContentTypeParser(
    "application/json",
    { parseAs: "string" },
    (_req, body, done) => {
      if (typeof body !== "string" || body.length === 0) return done(null, undefined);
      try {
        done(null, JSON.parse(body));
      } catch {
        done(null, undefined);
      }
    }
  );

  // 認証・管理者・デモの拒否も本番と同じフックを通す（ルートのハンドラはもう自分で断らない）。
  registerAccessControl(app);

  register(app);

  return app;
}

/** ログイン済みセッション。ルートが見るのは user.id と user.email だけ。 */
export const loggedInSession = {
  user: { id: "user-1", email: "user@example.com" },
};

/** デモアカウントのセッション（編集系は 403 で止まる）。 */
export const demoSession = {
  user: { id: "demo-1", email: "demo@juken-map.com" },
};

/**
 * ルートを叩いて応答を返す。
 *
 * app.inject() の戻り値はチェーン用の型と交差しており、そのままでは statusCode を
 * 引けない。応答の型を明示するのと、payload の型合わせをここで一度だけ行う。
 */
export function request(
  app: FastifyInstance,
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE",
  url: string,
  body?: unknown,
  headers?: Record<string, string>
): Promise<LightMyRequestResponse> {
  return app.inject({ method, url, payload: body as object, headers });
}

/**
 * これまでに書き出されたログを取り出して空にする。
 * テストの最初に呼んで前のテストの分を捨て、操作のあとにもう一度呼んで中身を確かめる。
 */
export function takeLogLines() {
  return testLogLines.splice(0);
}

/** テストの中で仮に作るルート用。入口の種類が無いと ready で失敗する（access-control.ts）。 */
export const publicRoute = { config: { access: "public" } } as const;
