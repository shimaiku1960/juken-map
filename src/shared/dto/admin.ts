// 管理者ページ（/admin）で API と画面が受け渡す形。日時は JSON で届くので ISO 文字列。
//
// 管理画面の API は Go が返す（JUK-78）ので、形の正は API の契約（openapi/openapi.yaml）で、ここは生成した型
// （openapi.gen.ts）に名前を付けて並べているだけ。形を変えるときは契約を直して `pnpm openapi:generate` を流す。

import type { components } from "@/shared/openapi.gen";

type Schemas = components["schemas"];

/**
 * 利用者の種別。合成ユーザーが実ユーザーの数字に混ざらないように分けて数える。
 *   real  実際の利用者
 *   sim   本番のシミュレーション（sim/）が作る合成ユーザー
 *   seed  手元の負荷検証用 seed（db/seed-synthetic.ts）が作る合成ユーザー
 *   demo  面接官向けのデモアカウント
 */
export type UserKind = Schemas["UserKind"];
/** 並びは画面の表示順。契約の enum と同じ値を並べる（型で食い違いを止める）。 */
export const USER_KINDS = ["real", "sim", "seed", "demo"] as const satisfies readonly UserKind[];

export type KindStats = Schemas["KindStats"];
export type AdminOverview = Schemas["AdminOverview"];
export type AdminUser = Schemas["AdminUser"];
export type AdminUserList = Schemas["AdminUserList"];
export type AdminUserRef = Schemas["AdminUserRef"];
export type AdminBanResult = Schemas["AdminBanResult"];
export type AdminDeleteResult = Schemas["AdminDeleteResult"];

// ---- マスター編集（/admin/masters） ----

export type AdminUniversity = Schemas["AdminUniversity"];
export type AdminUniversityList = Schemas["AdminUniversityList"];
export type AdminTag = Schemas["AdminTag"];
export type AdminFaculty = Schemas["AdminFaculty"];
export type AdminUniversityDetail = Schemas["AdminUniversityDetail"];
export type AdminFacultySnapshot = Schemas["AdminFacultySnapshot"];
export type AdminTextbookMaster = Schemas["AdminTextbookMaster"];

// ---- 障害注入（/admin の「障害注入」、JUK-178） ----

export type ChaosKind = Schemas["ChaosKind"];
export type ChaosExperiment = Schemas["ChaosExperiment"];
export type AdminChaosState = Schemas["AdminChaosState"];
