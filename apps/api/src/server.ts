import { existsSync } from "node:fs";
import path from "node:path";
import Fastify, { type FastifyInstance } from "fastify";
import fastifyCompress from "@fastify/compress";
import { toNodeHandler } from "better-auth/node";
import { registerAccessControl } from "./access-control.ts";
import { auth } from "./auth.ts";
import { BODY_LIMIT, registerErrorHandling } from "./error-handling.ts";
import { select, setQueryLogger } from "./infra/db.ts";
import { logger } from "./observability/logger.ts";
import { fastifyLoggingOptions } from "./observability/logger.ts";
import { registerMetrics, startMetricsServer } from "./observability/metrics.ts";
import { currentReqId, currentSim, runWithRequestContext } from "./observability/requestContext.ts";
import { DEFAULT_MAX_IN_FLIGHT, registerOverloadProtection } from "./overload.ts";
import { registerRoutes } from "./routes/index.ts";
import { registerSecurityHeaders } from "./security-headers.ts";
import { registerSpa } from "./spa.ts";

export async function buildServer() {
  // ログを切っていると、ハンドラで想定外の例外が起きて 500 を返しても、その中身は
  // どこにも残らない（Fastify の既定のエラー処理はロガーへ書くため）。
  const app = Fastify({ ...fastifyLoggingOptions, bodyLimit: BODY_LIMIT });

  // SQL を素の console.log ではなくロガーへ流す。こうすると reqId が付いて
  // どのリクエストが投げた SQL か分かり、LOG_FILE を設定していればファイルにも残る
  // （実際に書き出すかは db.ts 側の shouldLogQueries が開発中だけに絞る）。
  setQueryLogger((entry) => logger.info({ reqId: currentReqId(), sim: currentSim(), ...entry }));

  // 件数と所要時間は、下で横取りする Better Auth の分も含めて全リクエストで数える。
  registerMetrics(app);

  // エラー応答の形と reqId のヘッダー。圧縮プラグインより前に積んで onSend を先に走らせる。
  registerErrorHandling(app);

  // 混雑時に上限を超えた API を 503 で断る。断った分も件数に数えるよう metrics の後、
  // Better Auth のサインインも対象に含めるよう、下の横取りより前に積む。
  registerOverloadProtection(
    app,
    Number(process.env.OVERLOAD_MAX_IN_FLIGHT ?? DEFAULT_MAX_IN_FLIGHT)
  );

  // Next.js は応答を既定で圧縮していたが、Fastify は何もしない。SPA のバンドルは
  // 800KB 超あり、無圧縮のまま配ると回線の細い端末で目に見えて遅くなる。
  // nginx 側で gzip を足す手もあるが、本番ホストを触らない方針なのでアプリで持つ。
  //
  // ここは必ず await する。register() は遅延実行なので、待たずに下でルートを
  // 定義すると、プラグインが読み込まれる頃には既にルートが確定していて hook が
  // 掛からない。実際それで静的ファイル以外が無圧縮のままだった。
  await app.register(fastifyCompress, {
    global: true,
    encodings: ["br", "gzip", "deflate"],
  });

  // ここから先の処理（下の better-auth の横取りも含む）を、リクエストごとの文脈に入れる。
  // done() を runWithRequestContext の中で呼ぶと、そこから続く処理すべてが同じ文脈に入り、
  // measured() と db.ts が引数を受け取らずに reqId を読めるようになる。
  // 一番先に登録するのは、フックが登録順に走るため（better-auth が投げる SQL にも付く）。
  app.addHook("onRequest", (request, reply, done) => {
    // シミュレーションからのリクエストは、リクエストの行（request completed）にも印を付ける。
    const sim = request.headers["x-sim-run"] !== undefined;
    if (sim) request.log = reply.log = request.log.child({ sim: true });
    runWithRequestContext({ reqId: String(request.id), sim }, done);
  });

  // セキュリティヘッダーは、下で横取りする Better Auth の応答にも付くよう、その前に積む。
  registerSecurityHeaders(app);

  // better-auth は Node のリクエストストリームを自分で読む。
  // Fastify は既定で application/json を先に読み切ってしまうため、そのままだと
  // better-auth 側が空のボディを見て 400 になる（Phase 0 で実際に踏んだ）。
  // addContentTypeParser を parseAs: "string" で挟んでも同じくストリームが枯れる。
  //
  // onRequest はボディ解析より前に走るので、ここで hijack すればストリームは
  // 手つかずのまま better-auth へ渡る。
  app.addHook("onRequest", async (request, reply) => {
    if (!request.url.startsWith("/api/auth/")) return;
    reply.hijack();
    await toNodeHandler(auth)(request.raw, reply.raw);
  });

  // Fastify はハンドラに入る前にボディを解析するので、既定のままだと不正な JSON が
  // 認証チェックより先に 400 になる（Next.js は request.json() がハンドラ内なので
  // 401 が先に返る）。認証を先に効かせるため、解析に失敗しても例外にせず
  // body を undefined にして判断をハンドラへ委ねる。各ハンドラは Zod の safeParse で
  // 検証するので、認証を通った正当なリクエストなら undefined は 400 になる。
  app.addContentTypeParser(
    "application/json",
    { parseAs: "string" },
    (_req, body, done) => {
      if (typeof body !== "string" || body.length === 0) return done(null, undefined);
      try {
        done(null, JSON.parse(body));
      } catch {
        // 解析できない場合も 400 にせず undefined を渡す。
        // 認証されていないリクエストが JSON の中身で結果を知ることがないようにする。
        done(null, undefined);
      }
    }
  );

  // 全ルートの入口の種類（config.access）を見て、未ログイン・権限なし・デモの書き込みを
  // ハンドラより前にまとめて断る。種類の無いルートがあれば起動に失敗する。
  registerAccessControl(app);

  app.get("/api/health", { config: { access: "public" } }, async () => {
    // デプロイ後のスモークテストが叩く。DB に繋がらなければ 500 になり、
    // 前のイメージへ自動で戻る（.github/scripts/deploy-ec2.sh）。
    await select("SELECT 1");
    return { ok: true };
  });

  registerRoutes(app);

  const webDist = process.env.WEB_DIST_DIR;
  if (webDist && existsSync(path.join(webDist, "index.html"))) {
    registerSpa(app, webDist);
  }

  return app;
}

const isEntrypoint = process.argv[1]?.endsWith("server.ts");
if (isEntrypoint) {
  const port = Number(process.env.API_PORT ?? 4000);
  const app = await buildServer();
  // 待ち受け開始のログは Fastify が出す（"Server listening at ..."）。
  await app.listen({ port, host: "0.0.0.0" });

  if (process.env.METRICS_PORT) {
    startMetricsServer(Number(process.env.METRICS_PORT));
  }
}
