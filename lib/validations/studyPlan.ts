import { z } from "zod";
import { SUBJECT_VALUES } from "@/lib/subjects";

// 科目：固定リストの値 or null（未設定）
const subjectField = z
  .string()
  .refine((v) => SUBJECT_VALUES.includes(v), "科目の値が不正です")
  .nullable()
  .optional();

// 学習予定1件分（内容＋科目）
export const studyPlanItemSchema = z.object({
  content: z
    .string()
    .min(1, "内容を入力してください")
    .max(500, "500文字以内で入力してください")
    .trim(),
  subject: subjectField,
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

// 更新用（部分更新を許可）。date/content/subject/done を個別に更新できる。
export const updateStudyPlanSchema = z.object({
  date: z.string().min(1).optional(),
  content: z
    .string()
    .min(1, "内容を入力してください")
    .max(500, "500文字以内で入力してください")
    .trim()
    .optional(),
  subject: subjectField,
  done: z.boolean().optional(),
});

export type UpdateStudyPlanInput = z.infer<typeof updateStudyPlanSchema>;
