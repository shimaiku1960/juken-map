import { execute, select, transaction, type Db } from "@/api/infra/db";
import type { StudyLogRow } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";
import type { DailyStudyMinutes, StudyLog } from "@/shared/dto/study";
import { userDateConditions, type DateRange } from "./date-range.ts";
import {
  LOG_COLUMNS,
  TEXTBOOK_COLUMNS,
  pickTextbookDTO,
  type TextbookColumns,
} from "./study-columns.ts";

/**
 * 応答の件数の上限。期間で絞ったうえでの安全網で、ページングではない。
 * 新しい日付から詰めるので、超えたときに落ちるのは期間の古い側。
 */
const MAX_LOGS = 1000;

/**
 * 自分の実績の一覧を、画面へ返す形（src/shared/dto/study.ts の StudyLog）で返す。
 * 呼び出し元は GET /api/study-logs だけなので、日時もここで ISO 文字列にしておく。
 */
export function listStudyLogs(
  userId: string,
  range: DateRange
): Promise<StudyLog[]> {
  return measured("studyLog.list", async () => {
    const { where, params } = userDateConditions("l", userId, range);
    // Prisma の include は実績と参考書で SQL を2本に分けていた。LEFT JOIN 1本にする。
    //
    // 実績の日付は日単位なので、同じ日付の実績はよくある。ORDER BY date だけでは
    // その中の順番は決まらず、Prisma 版は DB が返した順（大半は記録した順、ときどき逆順）
    // だった。ヒートマップはその日の実績をこの順のまま並べるので、記録した順（id 昇順）に固定する。
    const rows = await select<StudyLogRow & TextbookColumns>(
      `SELECT ${LOG_COLUMNS}, ${TEXTBOOK_COLUMNS}
       FROM StudyLog AS l
       LEFT JOIN Textbook AS t ON t.id = l.textbookId
       WHERE ${where}
       ORDER BY l.date DESC, l.id ASC
       LIMIT ?`,
      [...params, MAX_LOGS]
    );
    return rows.map((row) => ({
      id: row.id,
      userId: row.userId,
      date: row.date.toISOString(),
      minutes: row.minutes,
      subject: row.subject,
      textbookId: row.textbookId,
      textbook: pickTextbookDTO(row),
      rangeStart: row.rangeStart,
      rangeEnd: row.rangeEnd,
      rangeUnit: row.rangeUnit,
      memo: row.memo,
      studyPlanId: row.studyPlanId,
      createdAt: row.createdAt.toISOString(),
      updatedAt: row.updatedAt.toISOString(),
    }));
  });
}

/**
 * 日ごとの合計学習時間を返す。連続記録日数（ストリーク）のように
 * 「その日に何分やったか」しか要らない画面のためのもの。
 *
 * 明細を全部返して画面側で合計していたのをこちらへ移した。1件あたり参考書もメモも
 * 範囲も付いてこないので、1年ぶんでも明細の数十分の1で済む。
 * (userId, date) の索引がそのまま GROUP BY に効くので、並べ替えも起きない。
 */
export function listDailyStudyMinutes(
  userId: string,
  range: DateRange
): Promise<DailyStudyMinutes[]> {
  return measured("studyLog.daily", async () => {
    const { where, params } = userDateConditions("l", userId, range);
    const rows = await select<{ date: Date; minutes: string }>(
      `SELECT l.date, SUM(l.minutes) AS minutes
       FROM StudyLog AS l
       WHERE ${where}
       GROUP BY l.date
       ORDER BY l.date DESC
       LIMIT ?`,
      [...params, MAX_LOGS]
    );
    return rows.map((row) => ({
      date: row.date.toISOString(),
      // SUM() は INT でも DECIMAL になり、mysql2 は文字列で返す。
      minutes: Number(row.minutes),
    }));
  });
}

// ここから下は書き込み。HTTP は知らない。

async function findStudyLogById(id: number, db?: Db) {
  const [row] = await select<StudyLogRow>(
    `SELECT ${LOG_COLUMNS} FROM StudyLog AS l WHERE l.id = ?`,
    [id],
    db
  );
  return row ?? null;
}

/** 更新・削除の前に「自分のものか」を確かめる。 */
export function findOwnedStudyLog(id: number, userId: string) {
  return measured("studyLog.findOwned", async () => {
    const [row] = await select<StudyLogRow>(
      `SELECT ${LOG_COLUMNS} FROM StudyLog AS l WHERE l.id = ? AND l.userId = ? LIMIT 1`,
      [id, userId]
    );
    return row ?? null;
  });
}

/**
 * 実績を1件記録する。「初回記録」の印付けと同じトランザクションで行う。
 *
 * UPDATE の WHERE に firstStudyLogAt IS NULL を入れて DB 側で判定させている。
 * 先に読んでから書くと、同時アクセスで両方が「初回」になり得る。
 */
export function createStudyLog(input: {
  userId: string;
  date: string;
  minutes: number;
  subject?: string | null;
  textbookId?: number | null;
  rangeStart?: number | null;
  rangeEnd?: number | null;
  rangeUnit?: string | null;
  memo?: string | null;
}) {
  return measured("studyLog.create", () =>
    transaction(async (tx) => {
      const now = new Date();
      const activation = await execute(
        `UPDATE \`user\` SET firstStudyLogAt = ?, updatedAt = ?
         WHERE id = ? AND firstStudyLogAt IS NULL`,
        [now, now, input.userId],
        tx
      );
      const inserted = await execute(
        `INSERT INTO StudyLog
           (userId, date, minutes, subject, textbookId,
            rangeStart, rangeEnd, rangeUnit, memo, createdAt, updatedAt)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        [
          input.userId,
          new Date(input.date),
          input.minutes,
          input.subject ?? null,
          input.textbookId ?? null,
          input.rangeStart ?? null,
          input.rangeEnd ?? null,
          input.rangeUnit ?? null,
          input.memo ?? null,
          now,
          now,
        ],
        tx
      );
      // INSERT は行を返さないので、応答に使う形を同じ tx で読み直す。
      const created = await findStudyLogById(inserted.insertId, tx);
      return { created: created!, isFirstStudyLog: activation.affectedRows === 1 };
    })
  );
}

/**
 * 実績を更新する。予定から作られた実績は、予定との紐づきを壊す項目
 * （日付・科目・参考書）を固定する。
 */
export function updateStudyLog(
  id: number,
  current: {
    studyPlanId: number | null;
    date: Date;
    subject: string | null;
    textbookId: number | null;
  },
  data: {
    date: string;
    minutes: number;
    subject?: string | null;
    textbookId?: number | null;
    rangeStart?: number | null;
    rangeEnd?: number | null;
    rangeUnit?: string | null;
    memo?: string | null;
  }
) {
  const fromPlan = current.studyPlanId != null;
  return measured("studyLog.update", async () => {
    await execute(
      `UPDATE StudyLog
       SET date = ?, minutes = ?, subject = ?, textbookId = ?,
           rangeStart = ?, rangeEnd = ?, rangeUnit = ?, memo = ?, updatedAt = ?
       WHERE id = ?`,
      [
        fromPlan ? current.date : new Date(data.date),
        data.minutes,
        fromPlan ? current.subject : data.subject ?? null,
        fromPlan ? current.textbookId : data.textbookId ?? null,
        data.rangeStart ?? null,
        data.rangeEnd ?? null,
        data.rangeUnit ?? null,
        data.memo ?? null,
        new Date(),
        id,
      ]
    );
    // MySQL の UPDATE は更新後の行を返さないので読み直す（Prisma も同じことをしていた）。
    const updated = await findStudyLogById(id);
    if (!updated) throw new Error(`StudyLog ${id} が見つかりません`);
    return updated;
  });
}

export function deleteStudyLog(id: number) {
  return measured("studyLog.delete", async () => {
    await execute("DELETE FROM StudyLog WHERE id = ?", [id]);
  });
}
