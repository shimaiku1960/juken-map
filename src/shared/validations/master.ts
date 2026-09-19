import { z } from "zod";
import { PREFECTURES } from "@/shared/prefectures";

// 管理者ページのマスター編集（/admin/masters）の入力。API（apps/api/src/routes/admin-masters.ts）と
// 画面のフォームで同じものを使う。

export const UNIVERSITY_TYPES = ["国立", "公立", "私立"] as const;

const name = (label: string) =>
  z
    .string()
    .trim()
    .min(1, `${label}を入力してください`)
    .max(100, `${label}は100文字以内で入力してください`);

export const universityInputSchema = z.object({
  name: name("大学名"),
  prefecture: z
    .string()
    .refine((value) => PREFECTURES.includes(value), "都道府県を選んでください"),
  type: z.enum(UNIVERSITY_TYPES, { error: "種別を選んでください" }),
});
export type UniversityInput = z.infer<typeof universityInputSchema>;

// 受験日は日付だけを扱う。保存は seed（db/seed.ts）と同じく new Date("YYYY-MM-DD")＝UTC の0時にそろえ、
// 既存の表示（受験日カウントダウンなど）の日付がずれないようにする。
const examDate = z
  .string()
  .regex(/^\d{4}-\d{2}-\d{2}$/, "受験日を選んでください")
  .refine((value) => !Number.isNaN(new Date(value).getTime()), "受験日を選んでください");

const tagIds = z
  .array(z.number().int().positive())
  .max(20, "タグは20個までです")
  .refine((ids) => new Set(ids).size === ids.length, "同じタグが重なっています");

export const facultyInputSchema = z.object({
  name: name("学部名"),
  examDate,
  tagIds,
});
export type FacultyInput = z.infer<typeof facultyInputSchema>;

export const createFacultySchema = facultyInputSchema.extend({
  universityId: z.number().int().positive(),
});
export type CreateFacultyInput = z.infer<typeof createFacultySchema>;
