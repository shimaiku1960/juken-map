import type { Prisma } from "@/app/generated/prisma/client";

// Prisma の戻り値（Date を持つ）を、画面が扱う形（ISO 文字列）へ翻訳する層。
//
// 同じデータが2経路で画面に届く:
//   1. Server Component が Prisma を直接呼び、props で渡す      → Date のまま
//   2. ブラウザが fetch("/api/...") で受け取る                   → JSON なので文字列
// JSON には Date 型がないので、型を1つに揃えるには文字列側へ寄せるしかない。
// その翻訳がかつて画面3箇所と API に散らばっていたため、ここへ集約した。
//
// 型もここに置く。app/hooks/ 側は re-export するだけなので、既存の import は変わらない。
// 変換関数だけを lib へ移すと lib が app/hooks の型を参照して依存が逆流するため、
// 型と変換をセットで置いている。

export type Textbook = {
  id: number;
  masterId: number | null;
  name: string;
  totalAmount: number | null;
  rangeUnit: string | null;
  targetDate: string | null;
  subject: string | null;
};

export type StudyPlan = {
  id: number;
  userId: string;
  date: string; // ISO 文字列（JSON 経由で来るため）
  content: string | null;
  subject: string | null;
  done: boolean;
  studyLogId: number | null;
  textbookId: number | null;
  textbook: Textbook | null; // include で取得（表示用。名前だけ使う）
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  createdAt: string;
  updatedAt: string;
};

export type StudyLog = {
  id: number;
  userId: string;
  date: string; // ISO 文字列（JSON 経由で来るため）
  minutes: number;
  subject: string | null;
  textbookId: number | null;
  textbook: Textbook | null; // include で取得（表示用。名前だけ使う）
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  memo: string | null;
  studyPlanId: number | null;
  createdAt: string;
  updatedAt: string;
};

// 変換元＝サービス層（listStudyPlans / listStudyLogs）の戻り値の要素。
// include の形を Prisma の型から導いているので、schema を変えると
// ここが型エラーになり、直し忘れに気づける。
type StudyPlanRow = Prisma.StudyPlanGetPayload<{
  include: { textbook: true; studyLog: { select: { id: true } } };
}>;

type StudyLogRow = Prisma.StudyLogGetPayload<{
  include: { textbook: true };
}>;

function toTextbookDTO(textbook: StudyPlanRow["textbook"]): Textbook | null {
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
