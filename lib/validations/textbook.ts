import { z } from "zod";
import { SUBJECT_VALUES } from "@/lib/subjects";

const textbookNameSchema = z
  .string()
  .trim()
  .min(1, "参考書名を入力してください")
  .max(100, "100文字以内で入力してください");

export const createTextbookSchema = z.union([
  z.object({
    name: textbookNameSchema,
    subject: z.enum(SUBJECT_VALUES as [string, ...string[]]).nullable().optional(),
    rangeUnit: z
      .enum(["page", "question", "chapter", "number", "part", "section"])
      .optional(),
  }),
  z.object({ masterId: z.number().int().positive() }),
]);

// 部分更新。送られてきた項目だけ更新する（逆算設定フォームは全項目を送るため
// 従来どおり動く。科目だけの更新など、一部項目だけの更新も許容する）。
export const updateTextbookProgressSchema = z.object({
  totalAmount: z
    .number()
    .int("整数で入力してください")
    .min(1, "1以上で入力してください")
    .max(100000, "100000以下で入力してください")
    .optional(),
  rangeUnit: z
    .enum(["page", "question", "chapter", "number", "part", "section"])
    .optional(),
  targetDate: z.iso.date().nullable().optional(),
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
