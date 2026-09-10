import Fastify, {
  type FastifyInstance,
  type LightMyRequestResponse,
} from "fastify";

/**
 * テスト用に、対象のルートだけを載せた Fastify を作る。
 *
 * buildServer() をそのまま使わないのは、全ルートを読み込むと microCMS や Resend の
 * クライアント生成が走り、テストに関係のない環境変数を要求するため。
 * JSON の解析だけは本番と同じ挙動にしておく（不正な JSON を 400 にせず undefined を
 * 渡し、認証チェックを先に効かせる。理由は server.ts のコメント参照）。
 */
export function buildTestApp(register: (app: FastifyInstance) => void) {
  const app = Fastify({ logger: false });

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
