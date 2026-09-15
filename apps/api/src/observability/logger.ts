import { randomUUID } from "node:crypto";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  LogController,
  type FastifyBaseLogger,
  type FastifyReply,
  type FastifyRequest,
} from "fastify";
import { pino, type LoggerOptions, type TransportTargetOptions } from "pino";
import { redactPath } from "./redact.ts";

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

function requestForLog(request: FastifyRequest) {
  // URL の ? 以降やパスに入るトークンは残さない（redact.ts）。
  return { method: request.method, url: redactPath(request.url) };
}

const level = process.env.LOG_LEVEL ?? "info";

const baseOptions: LoggerOptions = {
  level,
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

// リポジトリのルート。LOG_FILE の相対パスは、書く場所の .env と同じくここを基準にする
// （API はルートではなく apps/api をカレントにして動くため）。
const repoRoot = fileURLToPath(new URL("../../../../", import.meta.url));

/**
 * 手元の開発での書き出し先。
 *
 * 画面には pino-pretty で人が読める形を出す。LOG_FILE を設定したときは、本番と同じ
 * JSON をファイルにも書く。そのファイルを Alloy が読んで Loki へ送る（pnpm run obs:start）。
 * アプリから Loki へ直接送らないのは、送り先が落ちてもアプリに影響させないためと、
 * 本番へ持っていくときに Alloy の送り先を変えるだけで済むようにするため。
 */
export function developmentTargets(logFile: string | undefined): TransportTargetOptions[] {
  const targets: TransportTargetOptions[] = [
    {
      target: "pino-pretty",
      level,
      options: {
        translateTime: "SYS:HH:MM:ss",
        // リクエストの行は「GET /api/study-logs 200 12.3ms」、measured() の行は
        // 「studyLog.list 3.2ms success=true」の1行にまとめる。
        // 詳しく追いたいときは本番と同じ JSON（NODE_ENV=production）で見る。
        messageFormat:
          "{if req.method}{req.method} {req.url} {res.statusCode} {responseTime}ms {end}" +
          "{if operation}{operation} {duration_ms}ms success={success} {end}{msg}",
        ignore:
          "pid,hostname,reqId,req,res,responseTime,operation,duration_ms,success," +
          // トレースを有効にしたとき（instrumentation.ts）に足される。ファイルの JSON には残る。
          "trace_id,span_id,trace_flags",
      },
    },
  ];
  if (logFile) {
    targets.push({
      target: "pino/file",
      level,
      options: { destination: path.resolve(repoRoot, logFile), mkdir: true },
    });
  }
  return targets;
}

function createLogger() {
  if (isTest()) {
    return pino(baseOptions, {
      write: (line: string) => testLogLines.push(JSON.parse(line)),
    });
  }
  if (isProduction()) return pino(baseOptions);
  return pino({
    ...baseOptions,
    transport: { targets: developmentTargets(process.env.LOG_FILE) },
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
