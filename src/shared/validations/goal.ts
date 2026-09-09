import { z } from "zod";

// 志望校のステータス。candidate=気になる（比較検討中）/ decided=受験校として確定。
export const GOAL_STATUSES = ["candidate", "decided"] as const;
export const goalStatusSchema = z.enum(GOAL_STATUSES);
export type GoalStatus = z.infer<typeof goalStatusSchema>;

export const goalSchema = z.object({
  facultyId: z.number().int().positive("志望学部を選択してください"),
  // 未指定なら decided（志望校ページから直接追加＝受験校として扱う）
  status: goalStatusSchema.optional(),
});

export type GoalInput = z.infer<typeof goalSchema>;

export const updateGoalSchema = goalSchema.partial();

export type UpdateGoalInput = z.infer<typeof updateGoalSchema>;

// PATCH 用。第一志望トグルとメモ（ユーザー資産）をまとめて部分更新できる。
// isFirstChoice / note のどちらか、または両方を送る想定。
export const patchGoalSchema = z.object({
  isFirstChoice: z.boolean().optional(),
  note: z.string().max(500, "500文字以内で入力してください").nullable().optional(),
  status: goalStatusSchema.optional(),
});

export type PatchGoalInput = z.infer<typeof patchGoalSchema>;