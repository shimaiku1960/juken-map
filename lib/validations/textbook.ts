import { z } from "zod";

export const createTextbookSchema = z.object({
  name: z
    .string()
    .min(1, "参考書名を入力してください")
    .max(100, "100文字以内で入力してください")
    .trim(),
});

export type CreateTextbookInput = z.infer<typeof createTextbookSchema>;
