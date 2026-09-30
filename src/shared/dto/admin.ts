// 管理者ページ（/admin）で API と画面が受け渡す形。日時は JSON で届くので ISO 文字列。
//
// 利用者の管理（概要・一覧・停止・削除）は Go も返すので、形の正は API の契約（openapi/openapi.yaml）で、
// ここは生成した型（openapi.gen.ts）に名前を付けて並べているだけ（JUK-78）。形を変えるときは契約を直して
// `pnpm openapi:generate` を流す。マスター編集の型は、まだ Node だけが返すので下に手で書いている。

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

export type AdminUniversity = {
  id: number;
  name: string;
  prefecture: string;
  type: string;
  facultyCount: number;
  /** 配下の学部を志望校にしている件数。1件でもあれば大学は削除できない。 */
  goalCount: number;
};

export type AdminUniversityList = {
  universities: AdminUniversity[];
  total: number;
  page: number;
  pageSize: number;
};

export type AdminTag = { id: number; name: string };

export type AdminFaculty = {
  id: number;
  name: string;
  /** YYYY-MM-DD（UTC の0時で保存している日付部分）。 */
  examDate: string;
  tags: AdminTag[];
  /** この学部を志望校にしている件数。1件でもあれば削除できない。 */
  goalCount: number;
};

export type AdminUniversityDetail = {
  university: AdminUniversity;
  faculties: AdminFaculty[];
};

export type AdminTextbookMaster = {
  id: number;
  name: string;
  publisher: string | null;
  edition: string | null;
  isbn: string;
  metrics: { unit: string; totalAmount: number; isDefault: boolean }[];
  /** この参考書から登録された利用者の参考書の数。1冊でもあれば削除できない。 */
  textbookCount: number;
};
