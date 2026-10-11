import type { StudyLogParseResult } from "@/shared/dto/study";

// AI が一文から読み取った結果のうち、記録のフォームへ入れる値（JUK-180）。
//
// 読み取れなかった項目（null）は入れない。先に利用者が選んでいた値を、
// 「分からなかった」で消さないため。
export type QuickLogValues = {
  date?: string;
  minutes?: number;
  subject?: string;
  textbookId?: number;
};

export function quickLogValues(
  result: StudyLogParseResult,
  // 日付を決めて開いたフォーム（カレンダーの日から開いたとき）では、日付を変えない
  options: { dateFixed: boolean }
): QuickLogValues {
  const values: QuickLogValues = {};
  if (result.date != null && !options.dateFixed) values.date = result.date;
  if (result.minutes != null) values.minutes = result.minutes;
  if (result.subject != null) values.subject = result.subject;
  if (result.textbookId != null) values.textbookId = result.textbookId;
  return values;
}
