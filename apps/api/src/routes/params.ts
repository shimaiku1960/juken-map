import type { FastifyReply } from "fastify";
import { z } from "zod";

/**
 * path の `:id`（数値の主キー）。数字だけで書いた正の整数を受け付ける。
 *
 * Number() で済ませると "abc" が NaN になり、mysql2 の query が `WHERE id = NaN` という
 * SQL を組み立て、MySQL が「NaN という列は無い」と返して 500 になる。"1.5" や "1e3" も
 * 数値として通ってしまうので、変換する前に文字の形で絞る。15桁までにしているのは、
 * Number にしたときに誤差が出ない範囲に収めるため（主キーは INT なのでそれより小さい）。
 */
export const idParamsSchema = z.object({
  id: z.string().regex(/^[1-9][0-9]{0,14}$/).transform(Number),
});

/**
 * `:id` を読む。形が不正なら 400 を送って null を返す。
 * 呼び出し側は requireSession と同じく `if (id === null) return;` で抜ける。
 */
export function readIdParam(params: unknown, reply: FastifyReply): number | null {
  const parsed = idParamsSchema.safeParse(params);
  if (!parsed.success) {
    reply.code(400).send({ error: "ID が正しくありません" });
    return null;
  }
  return parsed.data.id;
}
