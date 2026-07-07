import { z } from "zod";

export const goalSchema = z.object({
  facultyId: z.number().int().positive("志望学部を選択してください"),
});

export type GoalInput = z.infer<typeof goalSchema>;

export const updateGoalSchema = goalSchema.partial();

export type UpdateGoalInput = z.infer<typeof updateGoalSchema>;

// PATCH 用。第一志望トグルとメモ（ユーザー資産）をまとめて部分更新できる。
// isFirstChoice / note のどちらか、または両方を送る想定。
export const patchGoalSchema = z.object({
  isFirstChoice: z.boolean().optional(),
  note: z.string().max(500, "500文字以内で入力してください").nullable().optional(),
});

export type PatchGoalInput = z.infer<typeof patchGoalSchema>;