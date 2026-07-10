import { z } from "zod";
import { SUBJECT_VALUES } from "@/lib/subjects";

// 範囲の単位（固定リスト）
export const RANGE_UNITS = [
  { value: "page", label: "ページ" },
  { value: "question", label: "問題" },
  { value: "chapter", label: "章" },
  { value: "number", label: "番" },
] as const;

export const RANGE_UNIT_VALUES: string[] = RANGE_UNITS.map((u) => u.value);

// 科目：固定リストの値 or null（未設定）
const subjectField = z
  .string()
  .refine((v) => SUBJECT_VALUES.includes(v), "科目の値が不正です")
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
      .refine((v) => RANGE_UNIT_VALUES.includes(v), "単位の値が不正です")
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
        path: [hasStart ? "rangeEnd" : "rangeStart"],
      });
    }

    // (b) 開始 ≦ 終了
    if (hasStart && hasEnd && val.rangeStart! > val.rangeEnd!) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "終了は開始以上にしてください",
        path: ["rangeEnd"],
      });
    }

    // (c) 範囲を入れたら単位必須
    if ((hasStart || hasEnd) && val.rangeUnit == null) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "単位を選択してください",
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
        path: ["content"],
      });
    }
  });

export type StudyPlanItemInput = z.infer<typeof studyPlanItemSchema>;

// 作成：1つの日付に複数の内容をまとめて登録する
export const createStudyPlansSchema = z.object({
  date: z.string().min(1, "日付を選択してください"), // "YYYY-MM-DD"
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
    date: z.string().min(1).optional(),
    textbookId: z.number().int().positive().nullable().optional(),
    rangeStart: z.number().int().positive().nullable().optional(),
    rangeEnd: z.number().int().positive().nullable().optional(),
    rangeUnit: z
      .string()
      .refine((v) => RANGE_UNIT_VALUES.includes(v), "単位の値が不正です")
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
        path: [hasStart ? "rangeEnd" : "rangeStart"],
      });
    }
    if (hasStart && hasEnd && val.rangeStart! > val.rangeEnd!) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "終了は開始以上にしてください",
        path: ["rangeEnd"],
      });
    }
    if ((hasStart || hasEnd) && val.rangeUnit == null) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "単位を選択してください",
        path: ["rangeUnit"],
      });
    }
  });

export type UpdateStudyPlanInput = z.infer<typeof updateStudyPlanSchema>;
