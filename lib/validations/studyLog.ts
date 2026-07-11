import { z } from "zod";
import { SUBJECT_VALUES } from "@/lib/subjects";
import { RANGE_UNIT_VALUES } from "@/lib/validations/studyPlan";

// 科目：固定リストの値 or null（未設定）
const subjectField = z
  .string()
  .refine((v) => SUBJECT_VALUES.includes(v), "科目の値が不正です")
  .nullable()
  .optional();

// 勉強「実績」1件分。時間（分）は必須、参考書＋範囲・メモ・科目は任意。
export const createStudyLogSchema = z
  .object({
    date: z.string().min(1, "日付を選択してください"), // "YYYY-MM-DD"
    minutes: z
      .number({ message: "学習時間を入力してください" })
      .int("整数で入力してください")
      .positive("1分以上を入力してください")
      .max(1440, "24時間（1440分）以内で入力してください"),
    subject: subjectField,
    textbookId: z.number().int().positive().nullable().optional(),
    rangeStart: z.number().int().positive().nullable().optional(),
    rangeEnd: z.number().int().positive().nullable().optional(),
    rangeUnit: z
      .string()
      .refine((v) => RANGE_UNIT_VALUES.includes(v), "単位の値が不正です")
      .nullable()
      .optional(),
    memo: z
      .string()
      .max(500, "500文字以内で入力してください")
      .trim()
      .optional(),
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
  });

export type CreateStudyLogInput = z.infer<typeof createStudyLogSchema>;
