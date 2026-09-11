import type {
  StudyLogRow,
  StudyPlanRow,
  TextbookRow,
} from "@/api/infra/tables";
import type { Textbook } from "@/shared/dto/study";

// 予定（p）・実績（l）・参考書（t）を SELECT するときの列と、JOIN の結果を
// オブジェクトへ戻す関数。study-plan-service と study-log-service の両方で使う。
//
// 列は * ではなく明示する。テーブルに列が増えても、返す形が勝手に変わらないように。
// 別名（p / l / t）は、使う側の FROM 句でこの名前を付けること。

export const PLAN_COLUMNS = `
  p.id, p.userId, p.date, p.content, p.subject, p.done, p.textbookId,
  p.rangeStart, p.rangeEnd, p.rangeUnit, p.createdAt, p.updatedAt
`;

export const LOG_COLUMNS = `
  l.id, l.userId, l.date, l.subject, l.minutes, l.textbookId,
  l.rangeStart, l.rangeEnd, l.rangeUnit, l.memo, l.studyPlanId,
  l.createdAt, l.updatedAt
`;

// JOIN すると予定・実績と参考書の列名（id, userId, createdAt...）がぶつかるので、
// 参考書側には tb_ を付けて区別し、あとで入れ子のオブジェクトへ組み直す。
export const TEXTBOOK_COLUMNS = `
  t.id AS tb_id, t.userId AS tb_userId, t.masterId AS tb_masterId,
  t.name AS tb_name, t.totalAmount AS tb_totalAmount, t.rangeUnit AS tb_rangeUnit,
  t.targetDate AS tb_targetDate, t.subject AS tb_subject,
  t.createdAt AS tb_createdAt, t.updatedAt AS tb_updatedAt
`;

export type TextbookColumns = {
  [K in keyof TextbookRow as `tb_${K}`]: TextbookRow[K] | null;
};

/**
 * JOIN で平らに並んだ tb_ 列を、参考書のオブジェクトへ戻す。
 * LEFT JOIN で相手が居なければ tb_ 列は全部 NULL になるので、そのときは null。
 *
 * Prisma の include はこの組み立てを自動でやっていた。
 */
export function pickTextbook(row: TextbookColumns): TextbookRow | null {
  if (row.tb_id === null) return null;
  return {
    id: row.tb_id,
    userId: row.tb_userId!,
    masterId: row.tb_masterId,
    name: row.tb_name!,
    totalAmount: row.tb_totalAmount,
    rangeUnit: row.tb_rangeUnit,
    targetDate: row.tb_targetDate,
    subject: row.tb_subject,
    createdAt: row.tb_createdAt!,
    updatedAt: row.tb_updatedAt!,
  };
}

/**
 * tb_ 列から、画面へ返す参考書の形（src/shared/dto/study.ts の Textbook）を作る。
 * 画面が使わない列（userId・作成日時）は落とし、日時は ISO 文字列にする。
 */
export function pickTextbookDTO(row: TextbookColumns): Textbook | null {
  if (row.tb_id === null) return null;
  return {
    id: row.tb_id,
    masterId: row.tb_masterId,
    name: row.tb_name!,
    totalAmount: row.tb_totalAmount,
    rangeUnit: row.tb_rangeUnit,
    targetDate: row.tb_targetDate?.toISOString() ?? null,
    subject: row.tb_subject,
  };
}

/** 予定の行から、JOIN で付いてきた余分な列を取り除く。 */
export function pickPlan(row: StudyPlanRow): StudyPlanRow {
  return {
    id: row.id,
    userId: row.userId,
    date: row.date,
    content: row.content,
    subject: row.subject,
    done: row.done,
    textbookId: row.textbookId,
    rangeStart: row.rangeStart,
    rangeEnd: row.rangeEnd,
    rangeUnit: row.rangeUnit,
    createdAt: row.createdAt,
    updatedAt: row.updatedAt,
  };
}

/** 実績の行から、JOIN で付いてきた余分な列を取り除く。 */
export function pickLog(row: StudyLogRow): StudyLogRow {
  return {
    id: row.id,
    userId: row.userId,
    date: row.date,
    subject: row.subject,
    minutes: row.minutes,
    textbookId: row.textbookId,
    rangeStart: row.rangeStart,
    rangeEnd: row.rangeEnd,
    rangeUnit: row.rangeUnit,
    memo: row.memo,
    studyPlanId: row.studyPlanId,
    createdAt: row.createdAt,
    updatedAt: row.updatedAt,
  };
}
