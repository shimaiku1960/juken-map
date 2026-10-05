import { z } from "zod";
import { SUBJECT_VALUES } from "@/shared/subjects";
import { isCalendarYmd, YMD_PATTERN } from "@/shared/date";

/**
 * 日付の文字列に「YYYY-MM-DD の形」と「暦にある日付」の規則を足す（JUK-75）。実績と予定の日付で使う。
 * 以前は形を見ずに new Date() へ渡していたので、"1" が 2001 年、"2026-09-3" がサーバーの時間帯の日付になり、
 * 空白だけでは 500 になっていた。Go（apps/api）も同じ規則で弾く。
 */
export function ymdDate(field: z.ZodString) {
  return field
    .regex(YMD_PATTERN, "日付は YYYY-MM-DD で指定してください")
    .refine(
      (date) => !YMD_PATTERN.test(date) || isCalendarYmd(date),
      { message: "存在しない日付です", params: { code: "invalid_date" } }
    );
}

// 範囲の単位（固定リスト）
export const RANGE_UNITS = [
  { value: "page", label: "ページ" },
  { value: "question", label: "問題" },
  { value: "chapter", label: "章" },
  { value: "number", label: "番" },
  { value: "part", label: "Part" },
  { value: "section", label: "Section" },
] as const;

export const RANGE_UNIT_VALUES: string[] = RANGE_UNITS.map((u) => u.value);

// 科目：固定リストの値 or null（未設定）
const subjectField = z
  .string()
  .refine((v) => SUBJECT_VALUES.includes(v), { message: "科目の値が不正です", params: { code: "invalid_subject" } })
  .nullable()
  .optional();

// 学習予定1件分（参考書＋範囲＋任意メモ＋科目）
export const studyPlanItemSchema = z
  .object({
    textbookId: z.number().int().positive().nullable().optional(),
    rangeStart: z.number().int().positive().nullable().optional(),
    rangeEnd: z.number().int().positive().nullable().optional(),
    rangeUnit: z
      .string()
      .refine((v) => RANGE_UNIT_VALUES.includes(v), { message: "単位の値が不正です", params: { code: "invalid_range_unit" } })
      .nullable()
      .optional(),
    content: z
      .string()
      .max(500, "500文字以内で入力してください")
      .trim()
      .optional(),
    subject: subjectField,
  })
  .superRefine((val, ctx) => {
    const hasStart = val.rangeStart != null;
    const hasEnd = val.rangeEnd != null;

    // (a) 開始と終了はセット
    if (hasStart !== hasEnd) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "範囲は開始と終了の両方を入力してください",
        params: { code: "range_incomplete" },
        path: [hasStart ? "rangeEnd" : "rangeStart"],
      });
    }

    // (b) 開始 ≦ 終了
    if (hasStart && hasEnd && val.rangeStart! > val.rangeEnd!) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "終了は開始以上にしてください",
        params: { code: "range_end_before_start" },
        path: ["rangeEnd"],
      });
    }

    // (c) 範囲を入れたら単位必須
    if ((hasStart || hasEnd) && val.rangeUnit == null) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "単位を選択してください",
        params: { code: "range_unit_required" },
        path: ["rangeUnit"],
      });
    }

    // (d) 中身ゼロ（参考書・範囲・メモが全部空）を禁止
    const empty =
      val.textbookId == null &&
      !hasStart &&
      !hasEnd &&
      (val.content == null || val.content === "");
    if (empty) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "参考書・範囲・メモのいずれかを入力してください",
        params: { code: "plan_content_required" },
        path: ["content"],
      });
    }
  });

export type StudyPlanItemInput = z.infer<typeof studyPlanItemSchema>;

// 作成：1つの日付に複数の内容をまとめて登録する
export const createStudyPlansSchema = z.object({
  date: ymdDate(z.string().min(1, "日付を選択してください")),
  items: z.array(studyPlanItemSchema).min(1, "内容を1つ以上入力してください"),
});

export type CreateStudyPlansInput = z.infer<typeof createStudyPlansSchema>;

// 単体作成互換（旧スキーマ。他で使っていれば残す）
export const studyPlanSchema = z.object({
  date: z.string().min(1, "日付を選択してください"),
  content: z
    .string()
    .min(1, "内容を入力してください")
    .max(500, "500文字以内で入力してください")
    .trim(),
});

export type StudyPlanInput = z.infer<typeof studyPlanSchema>;

// 更新用（部分更新を許可）。参考書・範囲・メモ・科目・完了を個別に更新できる。
export const updateStudyPlanSchema = z
  .object({
    date: ymdDate(z.string().min(1)).optional(),
    textbookId: z.number().int().positive().nullable().optional(),
    rangeStart: z.number().int().positive().nullable().optional(),
    rangeEnd: z.number().int().positive().nullable().optional(),
    rangeUnit: z
      .string()
      .refine((v) => RANGE_UNIT_VALUES.includes(v), { message: "単位の値が不正です", params: { code: "invalid_range_unit" } })
      .nullable()
      .optional(),
    content: z
      .string()
      .max(500, "500文字以内で入力してください")
      .trim()
      .optional(),
    subject: subjectField,
    done: z.boolean().optional(),
  })
  .superRefine((val, ctx) => {
    const hasStart = val.rangeStart != null;
    const hasEnd = val.rangeEnd != null;

    if (hasStart !== hasEnd) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "範囲は開始と終了の両方を入力してください",
        params: { code: "range_incomplete" },
        path: [hasStart ? "rangeEnd" : "rangeStart"],
      });
    }
    if (hasStart && hasEnd && val.rangeStart! > val.rangeEnd!) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "終了は開始以上にしてください",
        params: { code: "range_end_before_start" },
        path: ["rangeEnd"],
      });
    }
    if ((hasStart || hasEnd) && val.rangeUnit == null) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "単位を選択してください",
        params: { code: "range_unit_required" },
        path: ["rangeUnit"],
      });
    }
  });

export type UpdateStudyPlanInput = z.infer<typeof updateStudyPlanSchema>;
