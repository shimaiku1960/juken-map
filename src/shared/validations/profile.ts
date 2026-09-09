import { z } from "zod";

export const profileSchema = z.object({
  nickname: z
    .string({ message: "ニックネームは文字列で入力してください" })
    .min(1, "ニックネームは必須です")
    .max(50, "50文字以内で入力してください")
    .trim(),
});

export type ProfileInput = z.infer<typeof profileSchema>;