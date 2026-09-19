import { z } from "zod";
import { PREFECTURES } from "@/shared/prefectures";
import { RANGE_UNIT_VALUES } from "@/shared/validations/studyPlan";

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

// ---- 参考書 ----

const optionalText = (max: number) =>
  z
    .string()
    .trim()
    .max(max, `${max}文字以内で入力してください`)
    .nullable()
    .optional()
    .transform((value) => (value ? value : null));

// ハイフン・空白は取り除いて保存する（既存の ISBN は数字だけで入っている）。
// ISBN-10 の最後の桁は X になることがある。
const isbn = z
  .string()
  .transform((value) => value.replace(/[-\s]/g, "").toUpperCase())
  .refine((value) => /^\d{13}$/.test(value) || /^\d{9}[\dX]$/.test(value), "ISBN は10桁か13桁で入力してください");

const metric = z.object({
  unit: z.string().refine((value) => RANGE_UNIT_VALUES.includes(value), "単位を選んでください"),
  totalAmount: z
    .number({ error: "総量を入力してください" })
    .int("総量は整数で入力してください")
    .min(1, "総量は1以上で入力してください")
    .max(100000, "総量は100000以下で入力してください"),
  isDefault: z.boolean(),
});

export const textbookMasterInputSchema = z.object({
  name: z.string().trim().min(1, "参考書名を入力してください").max(150, "参考書名は150文字以内で入力してください"),
  publisher: optionalText(100),
  edition: optionalText(50),
  isbn,
  // 総量の候補（単位ごと）。利用者が参考書を登録したとき、isDefault の候補で総量が入る。
  metrics: z
    .array(metric)
    .min(1, "総量を1つ以上入力してください")
    .max(RANGE_UNIT_VALUES.length)
    .refine((items) => new Set(items.map((item) => item.unit)).size === items.length, "同じ単位が重なっています")
    .refine((items) => items.filter((item) => item.isDefault).length === 1, "既定の単位を1つ選んでください"),
});
export type TextbookMasterInput = z.infer<typeof textbookMasterInputSchema>;
