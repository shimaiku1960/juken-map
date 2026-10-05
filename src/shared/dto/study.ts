// フロントエンドとバックエンドの間で受け渡す形（DTO）の型定義。
//
// 形の正は API の契約（openapi/openapi.yaml）で、ここは生成した型（openapi.gen.ts）に名前を付けて
// 並べているだけ（JUK-76）。Go（apps/api）も同じ契約から型を作るので、形を変えるときは
// 契約を直して `pnpm openapi:generate` を流す。ここに型の中身を書き足さない。
//
// ここには型だけを置く。この形へ組み立てるのは apps/api（Go）の store
// （listStudyPlans / listStudyLogs）で、戻り値の型がそのままこの型になっている。
//
// 日付が Date ではなく ISO 文字列なのは、画面へは JSON で届き、JSON には Date 型がないため。
//
// 画面側の hooks（useStudyPlans / useStudyLogs）はこの型を re-export している。
// 画面と API の両方がこのファイルだけを見ればよい状態にしてある。
//
// 予定と実績の一覧には userId・createdAt・updatedAt を入れない。画面はどれも使わず、
// 自分のデータしか返らないので userId は全行同じ値になる。この3つで実績の一覧の
// 約3割のバイト数を占めていた（2026-09-25、JUK-49）。

import type { components } from "@/shared/openapi.gen";

type Schemas = components["schemas"];

export type Textbook = Schemas["Textbook"];

// date は ISO 文字列（JSON 経由で来るため）。textbook は JOIN で取得（表示用。名前だけ使う）
export type StudyPlan = Schemas["StudyPlan"];

// 両端を含む期間（"YYYY-MM-DD"）。サーバーが何を返したかを画面へ伝えるのに使う。
export type DateRange = Schemas["DateRange"];

// ダッシュボードの初回表示ぶん。1画面を1リクエストで賄うためにまとめた形。
// 期間はサーバーが決める（画面・シミュレーション・負荷試験で計算が散らばらないように）。
// 実績は過去だけなので当月＋直近7日、予定は未来にもあるので当月＋今週。
// どちらも画面側が用途ごとに絞って使う。
export type Dashboard = Schemas["Dashboard"];

// 日別の合計学習時間。ヒートマップの連続記録日数のように「その日に何分やったか」
// しか要らない画面のために、明細ではなくこの形で返す（1年ぶんでも数十KBに収まる）。
export type DailyStudyMinutes = Schemas["DailyStudyMinutes"];

export type StudyLog = Schemas["StudyLog"];
