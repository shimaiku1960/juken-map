import { z } from "zod";
import { SUBJECT_VALUES } from "@/lib/subjects";

const textbookNameSchema = z
  .string()
  .trim()
  .min(1, "参考書名を入力してください")
  .max(100, "100文字以内で入力してください");

export const createTextbookSchema = z.union([
  z.object({ name: textbookNameSchema }),
  z.object({ masterId: z.number().int().positive() }),
]);

export const updateTextbookProgressSchema = z.object({
  totalAmount: z
    .number()
    .int("整数で入力してください")
    .min(1, "1以上で入力してください")
    .max(100000, "100000以下で入力してください"),
  rangeUnit: z.enum([
    "page",
    "question",
    "chapter",
    "number",
    "part",
    "section",
  ]),
  targetDate: z.iso.date().nullable(),
  // 対策科目。未設定は null。送られてこなければ現状維持。
  subject: z
    .enum(SUBJECT_VALUES as [string, ...string[]])
    .nullable()
    .optional(),
});

export type CreateTextbookInput = z.infer<typeof createTextbookSchema>;
export type UpdateTextbookProgressInput = z.infer<
  typeof updateTextbookProgressSchema
>;
