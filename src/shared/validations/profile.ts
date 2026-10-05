import { z } from "zod";

export const profileSchema = z.object({
  nickname: z
    .string({ message: "ニックネームは文字列で入力してください" })
    // Zod は書いた順に確かめるので、trim を先にしないと空白だけが min(1) を通る（JUK-64）
    .trim()
    .min(1, "ニックネームは必須です")
    .max(50, "50文字以内で入力してください"),
});

export type ProfileInput = z.infer<typeof profileSchema>;