import type { FastifyInstance } from "fastify";
import { z } from "zod";
import { createStudyLogSchema } from "@/shared/validations/studyLog";
import { shiftYmd, todayYmdTokyo } from "@/shared/date";
import {
  createStudyLog,
  listDailyStudyMinutes,
  listStudyLogs,
} from "@/api/services/study-log-service";
import type { DateRange } from "@/api/services/date-range";
import { findOwnedTextbook } from "@/api/services/textbook-service";
import { textbookRangeError } from "@/api/domain/textbookRange";
import { currentSession } from "../access-control.ts";

// 期間を省いて呼ばれたときに遡る日数。画面はどれも明示して呼ぶので、これは
// 古いクライアントや手で叩いたときのための既定値。全期間を返す状態には戻さない。
const DEFAULT_LOG_DAYS = 90;
// 日別の合計は1件が小さいので、ストリークが途切れない限り遡れるよう1年取る。
const DEFAULT_DAILY_DAYS = 365;

const ymdField = z
  .string()
  .regex(/^\d{4}-\d{2}-\d{2}$/, "日付は YYYY-MM-DD で指定してください");

const rangeQuerySchema = z.object({
  from: ymdField.optional(),
  to: ymdField.optional(),
});

/**
 * ?from=&to= を期間にする。from を省いたら defaultDays ぶん遡り、to を省いたら上限なし。
 * 今日は日本時間で決める（利用者も通知も日本時間で動いている）。
 */
function toRange(
  query: z.infer<typeof rangeQuerySchema>,
  defaultDays: number
): DateRange {
  return {
    from: query.from ?? shiftYmd(todayYmdTokyo(), -(defaultDays - 1)),
    to: query.to,
  };
}

export function registerStudyLogRoutes(app: FastifyInstance) {
  app.get("/api/study-logs", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const parsed = rangeQuerySchema.safeParse(request.query);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    return listStudyLogs(session.user.id, toRange(parsed.data, DEFAULT_LOG_DAYS));
  });

  // 日ごとの合計だけを返す軽い方。ヒートマップの連続記録日数が使う。
  app.get("/api/study-logs/daily", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const parsed = rangeQuerySchema.safeParse(request.query);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    return listDailyStudyMinutes(
      session.user.id,
      toRange(parsed.data, DEFAULT_DAILY_DAYS)
    );
  });

  app.post("/api/study-logs", { config: { access: "user" } }, async (request, reply) => {
    const session = currentSession(request);

    const parsed = createStudyLogSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: parsed.error.issues });
    }

    // 参考書を指定する場合は、所有権と逆算設定との整合性を検証する。
    if (parsed.data.textbookId != null) {
      const textbook = await findOwnedTextbook(
        parsed.data.textbookId,
        session.user.id
      );
      if (!textbook) {
        return reply.code(400).send({ error: "不正な参考書です" });
      }

      const rangeError = textbookRangeError(textbook, parsed.data);
      if (rangeError) {
        return reply.code(400).send({ error: rangeError });
      }
    }

    const { created, isFirstStudyLog } = await createStudyLog({
      userId: session.user.id,
      ...parsed.data,
    });

    return reply.code(201).send({ ...created, isFirstStudyLog });
  });
}
