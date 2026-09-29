import type { FastifyReply } from "fastify";
import type { z } from "zod";

/**
 * 入力チェックで弾いたときの 400 の本文（JUK-76）。Go（apps/api-go の validationError）も同じ形を返す。
 *
 * - error：画面にそのまま出す文言（画面の api-client.ts はこれを表示する）
 * - code ：機械が読む種類。Node と Go のずれを確かめたり、画面が文言を持つ形へ移ったりするときに使う
 * - field：どの項目か（"rangeEnd"・"items.0.unit" のようにドットでつなぐ）。項目に結びつかなければ null
 *
 * Stripe（code・message・param）や GitHub（errors[].code・field）と同じく、コードと文言の両方を返す。
 * 以前は Zod の issues の配列をそのまま返していたが、画面が使っていたのは最初の1件の文言だけだった。
 */
export type ValidationErrorBody = {
  error: string;
  code: string;
  field: string | null;
};

/**
 * Zod の最初の issue を 400 の本文にする。
 *
 * code は、refine・superRefine で `params: { code: "range_end_before_start" }` のように付けた名前を使う。
 * 付いていなければ Zod の code（too_small・invalid_type・invalid_format など）をそのまま使う。
 * 組み込みのチェックは field と組み合わせれば区別できるので、名前を付けるのは自分で書いた規則だけでよい。
 */
export function validationErrorBody(error: z.ZodError): ValidationErrorBody {
  const [issue] = error.issues;
  const named = issue.code === "custom" ? issue.params?.code : undefined;
  return {
    error: issue.message,
    code: typeof named === "string" ? named : issue.code,
    field: issue.path.length > 0 ? issue.path.join(".") : null,
  };
}

/** 400 を送る。呼び出し側は `return sendValidationError(reply, parsed.error);` で抜ける。 */
export function sendValidationError(reply: FastifyReply, error: z.ZodError) {
  return reply.code(400).send(validationErrorBody(error));
}
