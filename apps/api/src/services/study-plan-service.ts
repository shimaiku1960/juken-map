import { execute, isDuplicateEntry, select, transaction, type Db } from "@/api/infra/db";
import { textbookRangeError } from "@/api/domain/textbookRange";
import type { StudyLogRow, StudyPlanRow, TextbookRow } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";
import type { StudyPlan } from "@/shared/dto/study";
import { userDateConditions, type DateRange } from "./date-range.ts";
import {
  LOG_COLUMNS,
  PLAN_COLUMNS,
  TEXTBOOK_COLUMNS,
  pickLog,
  pickPlan,
  pickTextbook,
  pickTextbookDTO,
  type TextbookColumns,
} from "./study-columns.ts";

// Prisma の include は、親を取ったあとに子を `WHERE id IN (...)` で別に取りに行く
// （予定・参考書・実績で SQL が3本）。ここでは LEFT JOIN 1本で済ませる。
// 実績は予定1件につき最大1件（StudyLog.studyPlanId が UNIQUE）なので、JOIN しても行は増えない。
//
// 1日に複数の予定を入れるのは普通なので、同じ日付の中は作った順（id 昇順）に固定する。
// ORDER BY date だけでは同じ日付の中の順番が決まらない（Prisma 版も DB 任せで、
// 既存データでは結果的に id 昇順になっていた）。
/**
 * 応答の件数の上限。期間で絞ったうえでの安全網で、ページングではない。
 * 予定は日付の昇順で返すので、超えたときに落ちるのは期間の新しい側。
 */
const MAX_PLANS = 1000;

/**
 * 自分の予定の一覧を、画面へ返す形（src/shared/dto/study.ts の StudyPlan）で返す。
 * 日時もここで ISO 文字列にしておく。
 *
 * 期間で絞るのは実績と同じ理由＝以前は全期間・全件を返していて、2026-09-23の実測では
 * 利用者あたり平均75.5KB あり、1画面の中でいちばん大きな応答になっていた。
 */
export function listStudyPlans(
  userId: string,
  range: DateRange
): Promise<StudyPlan[]> {
  return measured("studyPlan.list", async () => {
    const { where, params } = userDateConditions("p", userId, range);
    const rows = await select<
      StudyPlanRow & TextbookColumns & { log_id: number | null }
    >(
      `SELECT ${PLAN_COLUMNS}, ${TEXTBOOK_COLUMNS}, l.id AS log_id
       FROM StudyPlan AS p
       LEFT JOIN Textbook AS t ON t.id = p.textbookId
       LEFT JOIN StudyLog AS l ON l.studyPlanId = p.id
       WHERE ${where}
       ORDER BY p.date ASC, p.id ASC
       LIMIT ?`,
      [...params, MAX_PLANS]
    );

    return rows.map((row) => ({
      id: row.id,
      userId: row.userId,
      date: row.date.toISOString(),
      content: row.content,
      subject: row.subject,
      done: row.done,
      studyLogId: row.log_id,
      textbookId: row.textbookId,
      textbook: pickTextbookDTO(row),
      rangeStart: row.rangeStart,
      rangeEnd: row.rangeEnd,
      rangeUnit: row.rangeUnit,
      createdAt: row.createdAt.toISOString(),
      updatedAt: row.updatedAt.toISOString(),
    }));
  });
}

// ここから下は書き込み。HTTP は知らない。

async function findPlanById(id: number, db?: Db) {
  const [row] = await select<StudyPlanRow>(
    `SELECT ${PLAN_COLUMNS} FROM StudyPlan AS p WHERE p.id = ?`,
    [id],
    db
  );
  return row ?? null;
}

/** 更新・削除の前に「自分のものか」を確かめる。 */
export function findOwnedStudyPlan(id: number, userId: string) {
  return measured("studyPlan.findOwned", async () => {
    // 所有者の条件を WHERE に入れる。他人の予定は「存在しない」のと同じ扱いになる。
    const [row] = await select<StudyPlanRow>(
      `SELECT ${PLAN_COLUMNS} FROM StudyPlan AS p WHERE p.id = ? AND p.userId = ? LIMIT 1`,
      [id, userId]
    );
    return row ?? null;
  });
}

/** 完了処理用。参考書と、既に実績が紐づいているかを一度に引く。 */
function findOwnedStudyPlanForComplete(id: number, userId: string) {
  return measured("studyPlan.findOwnedForComplete", async () => {
    // Prisma 版は include: { studyLog: true } で実績の全列を取っていたが、
    // 使うのは「有るか無いか」だけなので id だけにした。
    const [row] = await select<
      StudyPlanRow & TextbookColumns & { log_id: number | null }
    >(
      `SELECT ${PLAN_COLUMNS}, ${TEXTBOOK_COLUMNS}, l.id AS log_id
       FROM StudyPlan AS p
       LEFT JOIN Textbook AS t ON t.id = p.textbookId
       LEFT JOIN StudyLog AS l ON l.studyPlanId = p.id
       WHERE p.id = ? AND p.userId = ?
       LIMIT 1`,
      [id, userId]
    );
    if (!row) return null;
    return {
      ...pickPlan(row),
      textbook: pickTextbook(row),
      studyLog: row.log_id === null ? null : { id: row.log_id },
    };
  });
}

/** 1つの日付に複数の予定をまとめて作る。 */
export function createStudyPlans(input: {
  userId: string;
  date: Date;
  items: {
    textbookId?: number | null;
    rangeStart?: number | null;
    rangeEnd?: number | null;
    rangeUnit?: string | null;
    content?: string | null;
    subject?: string | null;
  }[];
}) {
  return measured("studyPlan.createMany", async () => {
    if (input.items.length === 0) return { count: 0 };

    // 作成・更新日時はアプリ側で入れる。Prisma の @updatedAt は DB の機能ではなく
    // Prisma が毎回値を足していたもので、テーブルに既定値は無い（書かないとエラー）。
    const now = new Date();
    // 複数行を1本の INSERT で入れる。(?, ?, ...) を行の数だけ並べる。
    const placeholders = input.items
      .map(() => "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
      .join(", ");
    const params = input.items.flatMap((item) => [
      input.userId,
      input.date,
      item.content ?? null,
      item.subject ?? null,
      item.textbookId ?? null,
      item.rangeStart ?? null,
      item.rangeEnd ?? null,
      item.rangeUnit ?? null,
      now,
      now,
    ]);

    const result = await execute(
      `INSERT INTO StudyPlan
         (userId, date, content, subject, textbookId,
          rangeStart, rangeEnd, rangeUnit, createdAt, updatedAt)
       VALUES ${placeholders}`,
      params
    );
    return { count: result.affectedRows };
  });
}

/** この予定に紐づく実績の件数。完了を取り消してよいかの判断に使う。 */
async function countLinkedStudyLogs(planId: number) {
  const [row] = await select<{ count: number }>(
    "SELECT COUNT(*) AS count FROM StudyLog WHERE studyPlanId = ?",
    [planId]
  );
  // COUNT(*) は BIGINT。ドライバの設定によっては文字列や BigInt で返るので数値に揃える。
  return Number(row.count);
}

/**
 * 送られてきた項目だけを更新する。
 *
 * 実績を記録済みの予定は未完了へ戻せない（戻すと実績だけが宙に浮く）。
 * このルールはどの入口から呼んでも効くよう、ここで判定する。
 */
export function updateStudyPlan(
  id: number,
  data: {
    date?: string;
    content?: string | null;
    subject?: string | null;
    textbookId?: number | null;
    rangeStart?: number | null;
    rangeEnd?: number | null;
    rangeUnit?: string | null;
    done?: boolean;
  }
) {
  return measured("studyPlan.update", async (): Promise<UpdateOutcome> => {
    if (data.done === false && (await countLinkedStudyLogs(id)) > 0) {
      return { result: "has_log" };
    }

    // 「送られてきた列だけ SET する」を自分で組み立てる。
    // 列名はこのコードに書いた固定の名前だけで、利用者の入力は値として ? で渡す。
    const changes: [column: string, value: unknown][] = [];
    if (data.date) changes.push(["date", new Date(data.date)]);
    if (data.content !== undefined) changes.push(["content", data.content]);
    if (data.subject !== undefined) changes.push(["subject", data.subject]);
    if (data.textbookId !== undefined) changes.push(["textbookId", data.textbookId]);
    if (data.rangeStart !== undefined) changes.push(["rangeStart", data.rangeStart]);
    if (data.rangeEnd !== undefined) changes.push(["rangeEnd", data.rangeEnd]);
    if (data.rangeUnit !== undefined) changes.push(["rangeUnit", data.rangeUnit]);
    if (data.done !== undefined) changes.push(["done", data.done]);
    changes.push(["updatedAt", new Date()]);

    await execute(
      `UPDATE StudyPlan SET ${changes.map(([column]) => `${column} = ?`).join(", ")}
       WHERE id = ?`,
      [...changes.map(([, value]) => value), id]
    );

    // MySQL の UPDATE は更新後の行を返さない（PostgreSQL の RETURNING が無い）。
    // Prisma もここで SELECT をもう1本流していた。
    const updated = await findPlanById(id);
    if (!updated) throw new Error(`StudyPlan ${id} が見つかりません`);
    return { result: "ok", value: updated };
  });
}

type UpdateOutcome = { result: "ok"; value: StudyPlanRow } | { result: "has_log" };

export function deleteStudyPlan(id: number) {
  return measured("studyPlan.delete", async () => {
    // 紐づく実績の studyPlanId は、外部キーの ON DELETE SET NULL で DB が NULL にする。
    await execute("DELETE FROM StudyPlan WHERE id = ?", [id]);
  });
}

type CompleteOutcome =
  | {
      result: "ok";
      value: {
        log: StudyLogRow & { textbook: TextbookRow | null };
        plan: StudyPlanRow;
        isFirstStudyLog: boolean;
      };
    }
  | { result: "not_found" }
  | { result: "already_completed" }
  | { result: "invalid_range"; message: string };

/**
 * 自分の予定を完了にし、実績を1件作る。完了に関するルールはすべてここで判定する。
 *
 * - 他人の予定・存在しない予定は not_found（区別しない）
 * - 実績が既にあれば already_completed。同時に完了した場合も一意制約で同じ結果になる
 * - 範囲は送られてきたものを優先し、無ければ予定の値を使う。参考書の逆算設定と
 *   噛み合わなければ invalid_range（実績の記録と同じ規則 `textbookRangeError`）
 */
export async function completeOwnedStudyPlan(input: {
  userId: string;
  planId: number;
  minutes: number;
  rangeStart?: number | null;
  rangeEnd?: number | null;
  rangeUnit?: string | null;
  memo?: string | null;
}): Promise<CompleteOutcome> {
  const plan = await findOwnedStudyPlanForComplete(input.planId, input.userId);
  if (!plan) return { result: "not_found" };
  if (plan.studyLog) return { result: "already_completed" };

  const rangeStart = input.rangeStart !== undefined ? input.rangeStart : plan.rangeStart;
  const rangeEnd = input.rangeEnd !== undefined ? input.rangeEnd : plan.rangeEnd;
  const rangeUnit = input.rangeUnit !== undefined ? input.rangeUnit : plan.rangeUnit;

  if (plan.textbook) {
    const message = textbookRangeError(plan.textbook, { rangeEnd, rangeUnit });
    if (message) return { result: "invalid_range", message };
  }

  try {
    const { log, updatedPlan, isFirstStudyLog } = await completeStudyPlan({
      userId: input.userId,
      plan,
      minutes: input.minutes,
      rangeStart: rangeStart ?? null,
      rangeEnd: rangeEnd ?? null,
      rangeUnit: rangeUnit ?? null,
      memo: input.memo ?? null,
    });
    return { result: "ok", value: { log, plan: updatedPlan, isFirstStudyLog } };
  } catch (error) {
    // 同じ予定を同時に完了すると一意制約に当たる。これは「すでに記録済み」と同じ。
    if (isDuplicateEntry(error)) return { result: "already_completed" };
    throw error;
  }
}

/**
 * 予定を完了にし、同時に実績を1件作る。
 *
 * 実績の作成と予定の完了は必ず一緒に成立させる。片方だけ成功すると
 * 「完了なのに実績が無い」または「実績はあるのに未完了」という状態が残る。
 *
 * 「初回記録」の印は UPDATE の WHERE に firstStudyLogAt IS NULL を入れて
 * DB 側で判定させる。先に読んでから書くと、同時アクセスで両方が初回になり得る。
 */
function completeStudyPlan(input: {
  userId: string;
  plan: { id: number; date: Date; subject: string | null; textbookId: number | null };
  minutes: number;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  memo: string | null;
}) {
  return measured("studyPlan.complete", () =>
    transaction(async (tx) => {
      const now = new Date();

      // 条件に合う行が無ければ何も変わらず affectedRows は 0。1なら今回が初回。
      const activation = await execute(
        `UPDATE \`user\` SET firstStudyLogAt = ?, updatedAt = ?
         WHERE id = ? AND firstStudyLogAt IS NULL`,
        [now, now, input.userId],
        tx
      );

      // 同じ予定の実績が既にあれば、studyPlanId の UNIQUE 制約で ER_DUP_ENTRY になり、
      // transaction() が ROLLBACK して例外を投げ直す。completeOwnedStudyPlan が already_completed に読み替える。
      const inserted = await execute(
        `INSERT INTO StudyLog
           (userId, studyPlanId, date, minutes, subject, textbookId,
            rangeStart, rangeEnd, rangeUnit, memo, createdAt, updatedAt)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        [
          input.userId,
          input.plan.id,
          input.plan.date,
          input.minutes,
          input.plan.subject,
          input.plan.textbookId,
          input.rangeStart,
          input.rangeEnd,
          input.rangeUnit,
          input.memo,
          now,
          now,
        ],
        tx
      );

      await execute(
        "UPDATE StudyPlan SET done = ?, updatedAt = ? WHERE id = ?",
        [true, now, input.plan.id],
        tx
      );

      // INSERT も UPDATE も行を返さないので、応答に使う形を読み直す。
      // 同じ tx で読むので、まだ確定していない自分の変更が見える。
      const [logRow] = await select<StudyLogRow & TextbookColumns>(
        `SELECT ${LOG_COLUMNS}, ${TEXTBOOK_COLUMNS}
         FROM StudyLog AS l
         LEFT JOIN Textbook AS t ON t.id = l.textbookId
         WHERE l.id = ?`,
        [inserted.insertId],
        tx
      );
      const updatedPlan = await findPlanById(input.plan.id, tx);

      return {
        log: { ...pickLog(logRow), textbook: pickTextbook(logRow) },
        updatedPlan: updatedPlan!,
        isFirstStudyLog: activation.affectedRows === 1,
      };
    })
  );
}
