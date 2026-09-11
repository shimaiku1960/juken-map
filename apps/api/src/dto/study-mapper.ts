import type {
  StudyLogRow as StudyLogTableRow,
  StudyPlanRow as StudyPlanTableRow,
  TextbookRow,
} from "@/api/infra/tables";

import type { StudyLog, StudyPlan, Textbook } from "@/shared/dto/study";

// Prisma の戻り値（Date を持つ）を、画面が扱う形（ISO 文字列）へ翻訳する。
//
// 同じデータが2経路で画面に届く:
//   1. Server Component が Prisma を直接呼び、props で渡す      → Date のまま
//   2. ブラウザが fetch("/api/...") で受け取る                   → JSON なので文字列
// JSON には Date 型がないので、型を1つに揃えるには文字列側へ寄せるしかない。
// その翻訳がかつて画面3箇所と API に散らばっていたため、ここへ集約した。
// 受け渡す形そのもの（型）は shared 側にある。

// 変換元＝サービス層（listStudyPlans / listStudyLogs）の戻り値の要素。
// 以前は Prisma の型から導いていたので schema の変更が型エラーで分かったが、
// 今は infra/tables.ts の手書きの型が頼り。
type StudyPlanRow = StudyPlanTableRow & {
  textbook: TextbookRow | null;
  studyLog: { id: number } | null;
};

type StudyLogRow = StudyLogTableRow & { textbook: TextbookRow | null };

function toTextbookDTO(textbook: TextbookRow | null): Textbook | null {
  if (!textbook) return null;
  return {
    id: textbook.id,
    masterId: textbook.masterId,
    name: textbook.name,
    totalAmount: textbook.totalAmount,
    rangeUnit: textbook.rangeUnit,
    targetDate: textbook.targetDate?.toISOString() ?? null,
    subject: textbook.subject,
  };
}

export function toStudyPlanDTO(plan: StudyPlanRow): StudyPlan {
  return {
    id: plan.id,
    userId: plan.userId,
    date: plan.date.toISOString(),
    content: plan.content,
    subject: plan.subject,
    done: plan.done,
    studyLogId: plan.studyLog?.id ?? null,
    textbookId: plan.textbookId,
    textbook: toTextbookDTO(plan.textbook),
    rangeStart: plan.rangeStart,
    rangeEnd: plan.rangeEnd,
    rangeUnit: plan.rangeUnit,
    createdAt: plan.createdAt.toISOString(),
    updatedAt: plan.updatedAt.toISOString(),
  };
}

export function toStudyLogDTO(log: StudyLogRow): StudyLog {
  return {
    id: log.id,
    userId: log.userId,
    date: log.date.toISOString(),
    minutes: log.minutes,
    subject: log.subject,
    textbookId: log.textbookId,
    textbook: toTextbookDTO(log.textbook),
    rangeStart: log.rangeStart,
    rangeEnd: log.rangeEnd,
    rangeUnit: log.rangeUnit,
    memo: log.memo,
    studyPlanId: log.studyPlanId,
    createdAt: log.createdAt.toISOString(),
    updatedAt: log.updatedAt.toISOString(),
  };
}
