import type { FastifyInstance } from "fastify";

/**
 * JSON ボディの上限。Fastify の既定と同じ 1MiB だが、既定値に任せると
 * バージョン更新で静かに変わるため明示する。超えた分はハンドラへ入る前に 413 で止まる。
 */
export const BODY_LIMIT = 1024 * 1024;

/**
 * 利用者に見せて安全な文言。Fastify が付ける英語メッセージは内部の都合
 * （パーサー名や上限値）が混ざるので、そのまま画面へ出さない。
 */
const CLIENT_MESSAGES: Record<string, string> = {
  FST_ERR_CTP_BODY_TOO_LARGE: "送信されたデータが大きすぎます",
  FST_ERR_CTP_INVALID_MEDIA_TYPE: "この形式のデータは受け取れません",
};

const FALLBACK_CLIENT_MESSAGE = "リクエストを処理できませんでした";
const SERVER_MESSAGE = "サーバー側で問題が発生しました";
const OVERLOADED_MESSAGE = "ただいま混み合っています。少し待ってからもう一度お試しください";

/** エラー応答の形。フロントの api-client.ts が読むのは error だけで、残りは調査用。 */
export type ErrorBody = {
  error: string;
  code: string;
  reqId: string;
};

export function errorBody(statusCode: number, code: string, reqId: string): ErrorBody {
  return {
    error:
      code === "OVERLOADED"
        ? OVERLOADED_MESSAGE
        : statusCode >= 500
        ? SERVER_MESSAGE
        : (CLIENT_MESSAGES[code] ?? FALLBACK_CLIENT_MESSAGE),
    code,
    reqId,
  };
}

export function registerErrorHandling(app: FastifyInstance) {
  // reqId を応答ヘッダーに載せる。
  //
  // 本文に入れる方法だと、本文を持たない 204 や JSON でない応答を拾えず、さらに
  // ルートが個別に返している 4xx（{error: string}）を全部書き換えることになる。
  // ヘッダーなら1か所で済み、調査のときは Network タブの値でそのままログを引ける。
  //
  // 静的ファイルの応答には付けない（assets は常時キャッシュで、付けても使わない）。
  app.addHook("onSend", (request, reply, _payload, done) => {
    if (reply.statusCode >= 400 || request.url.startsWith("/api/")) {
      reply.header("x-request-id", String(request.id));
    }
    done();
  });

  // 既定のエラー処理は 5xx の本文に err.message をそのまま入れるため、SQL や
  // ファイルパスが利用者へ出る。5xx は固定文言に差し替え、原因はログだけに残す。
  app.setErrorHandler((error, request, reply) => {
    // このハンドラの error は unknown で渡る（何が throw されるか分からないため）。
    // Fastify 自身のエラーは statusCode と code を持つので、その2つだけ取り出す。
    const thrown = (error ?? {}) as { statusCode?: unknown; code?: unknown };
    const statusCode = typeof thrown.statusCode === "number" ? thrown.statusCode : 500;
    const code =
      typeof thrown.code === "string" && thrown.code !== ""
        ? thrown.code
        : statusCode >= 500
          ? "INTERNAL_ERROR"
          : "BAD_REQUEST";

    // 5xx は想定外なので必ず残す。4xx は送信側の問題なので warn に留める。
    if (statusCode >= 500) {
      request.log.error({ err: error, statusCode, code }, "request failed");
    } else {
      request.log.warn({ err: error, statusCode, code }, "request rejected");
    }

    return reply.code(statusCode).send(errorBody(statusCode, code, String(request.id)));
  });
}
