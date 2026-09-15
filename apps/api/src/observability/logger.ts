import { randomUUID } from "node:crypto";
import {
  LogController,
  type FastifyBaseLogger,
  type FastifyReply,
  type FastifyRequest,
} from "fastify";
import { pino, type LoggerOptions } from "pino";

// サーバー全体で使うロガー。Fastify にも loggerInstance として渡すので、
// ルートの request.log はこれの子になり、同じ出力先・同じ設定で reqId が付く。
// リクエストの外（Better Auth のメール送信フックなど）ではこれを直接使う。
//
// 本番は1行1件の JSON（CloudWatch などで検索・集計できる形）、手元の開発は
// pino-pretty で人が読める形。テストは画面に出さず、下の testLogLines に溜める。

function isProduction() {
  return process.env.NODE_ENV === "production";
}

function isTest() {
  return process.env.NODE_ENV === "test";
}

/**
 * ログに残す URL からトークンを取り除く。
 *
 * メール確認・パスワード再設定のリンクや OAuth の戻り先は、URL の ? 以降に
 * トークンや code が乗る。そのままログに残すと、ログを読める人がそのリンクを
 * 使えてしまう。パスワード再設定だけはトークンがパスに入る
 * （Better Auth の /api/auth/reset-password/:token）ので、そこも伏せる。
 */
export function pathForLog(url: string) {
  const [path] = url.split("?");
  return path.replace(/^(\/api\/auth\/reset-password\/)[^/]+/, "$1:token");
}

function requestForLog(request: FastifyRequest) {
  return { method: request.method, url: pathForLog(request.url) };
}

const baseOptions: LoggerOptions = {
  level: process.env.LOG_LEVEL ?? "info",
  serializers: {
    // 既定の req には接続元 IP なども入るが、nginx 越しなので常に nginx の値になり
    // 役に立たない。何のリクエストかが分かれば足りるので、メソッドとパスだけにする。
    req: requestForLog,
  },
};

/**
 * テストで書き出されたログ（1行を JSON として読んだもの）。テスト以外では空のまま。
 *
 * テストは logger を spy するのではなく、ここを読んで確かめる。Fastify はリクエストごとの
 * 子ロガーにレベルを設定し直すので、子は親のメソッドを引き継がず、親の spy が効かない。
 * 書き出された後の形を見るので、URL からトークンが消えているかまで確かめられる。
 */
export const testLogLines: Record<string, unknown>[] = [];

function createLogger() {
  if (isTest()) {
    return pino(baseOptions, {
      write: (line: string) => testLogLines.push(JSON.parse(line)),
    });
  }
  if (isProduction()) return pino(baseOptions);
  return pino({
    ...baseOptions,
    transport: {
      target: "pino-pretty",
      options: {
        translateTime: "SYS:HH:MM:ss",
        // リクエストの行は「GET /api/study-logs 200 12.3ms」、measured() の行は
        // 「studyLog.list 3.2ms success=true」の1行にまとめる。
        // 詳しく追いたいときは本番と同じ JSON（NODE_ENV=production）で見る。
        messageFormat:
          "{if req.method}{req.method} {req.url} {res.statusCode} {responseTime}ms {end}" +
          "{if operation}{operation} {duration_ms}ms success={success} {end}{msg}",
        ignore:
          "pid,hostname,reqId,req,res,responseTime,operation,duration_ms,success",
      },
    },
  });
}

export const logger = createLogger();

/**
 * リクエストのログを「完了時に1行」にまとめる。
 *
 * Fastify の既定は受け付けたときと返したときの2行で、返したときの行には URL が無い。
 * 1行で「何に・何を返し・何ms かかったか」が分かるよう、受け付けの行は debug に下げ、
 * 完了の行に method と url を足す。
 */
class RequestLogController extends LogController {
  incomingRequest(request: FastifyRequest) {
    request.log.debug({ req: request }, "incoming request");
  }

  requestCompleted(
    error: Error | null | undefined,
    request: FastifyRequest,
    reply: FastifyReply
  ) {
    const fields = {
      req: request,
      res: reply,
      responseTime: Number(reply.elapsedTime.toFixed(1)),
    };
    if (error) {
      reply.log.error({ ...fields, err: error }, "request errored");
    } else {
      reply.log.info(fields, "request completed");
    }
  }
}

/** Fastify() に渡すログ関係の設定。本番もテストも同じものを使う。 */
export const fastifyLoggingOptions = {
  // pino の型のまま渡すと、Fastify インスタンスの型が「pino のロガーを持つもの」に変わり、
  // FastifyInstance（既定のロガー型）を受け取る各ルートの登録関数と合わなくなる。
  loggerInstance: logger as FastifyBaseLogger,
  logController: new RequestLogController(),
  // 既定の reqId は "req-1" からの連番で、再起動のたびに振り直しになる。
  // デプロイをまたいでログを突き合わせても取り違えないよう、UUID にする。
  genReqId: () => randomUUID(),
};
