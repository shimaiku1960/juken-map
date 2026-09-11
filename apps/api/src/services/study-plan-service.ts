import { execute, select, transaction, type Db } from "@/api/infra/db";
import type { StudyLogRow, StudyPlanRow } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";
import type { StudyPlan } from "@/shared/dto/study";
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
const LIST_SQL = `
  SELECT ${PLAN_COLUMNS}, ${TEXTBOOK_COLUMNS}, l.id AS log_id
  FROM StudyPlan AS p
  LEFT JOIN Textbook AS t ON t.id = p.textbookId
  LEFT JOIN StudyLog AS l ON l.studyPlanId = p.id
  WHERE p.userId = ?
  ORDER BY p.date ASC, p.id ASC
`;

/**
 * 自分の予定の一覧を、画面へ返す形（src/shared/dto/study.ts の StudyPlan）で返す。
 * 呼び出し元は GET /api/study-plans だけなので、日時もここで ISO 文字列にしておく。
 */
export function listStudyPlans(userId: string): Promise<StudyPlan[]> {
  return measured("studyPlan.list", async () => {
    const rows = await select<
      StudyPlanRow & TextbookColumns & { log_id: number | null }
    >(LIST_SQL, [userId]);

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
export function findOwnedStudyPlanForComplete(id: number, userId: string) {
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
export function countLinkedStudyLogs(planId: number) {
  return measured("studyPlan.countLinkedLogs", async () => {
    const [row] = await select<{ count: number }>(
      "SELECT COUNT(*) AS count FROM StudyLog WHERE studyPlanId = ?",
      [planId]
    );
    // COUNT(*) は BIGINT。ドライバの設定によっては文字列や BigInt で返るので数値に揃える。
    return Number(row.count);
  });
}

/** 送られてきた項目だけを更新する。 */
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
  return measured("studyPlan.update", async () => {
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
    return updated;
  });
}

export function deleteStudyPlan(id: number) {
  return measured("studyPlan.delete", async () => {
    // 紐づく実績の studyPlanId は、外部キーの ON DELETE SET NULL で DB が NULL にする。
    await execute("DELETE FROM StudyPlan WHERE id = ?", [id]);
  });
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
export function completeStudyPlan(input: {
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
      // transaction() が ROLLBACK して例外を投げ直す。ルートがそれを 409 に翻訳する。
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
