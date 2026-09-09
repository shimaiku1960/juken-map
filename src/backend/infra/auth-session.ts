import { cache } from "react";
import { headers } from "next/headers";
import { auth } from "@/backend/infra/auth";
import { measured } from "@/backend/observability/measured";

/**
 * Server Component 用のセッション取得。
 *
 * better-auth の getSession() は呼ぶたびに session と user を1本ずつ SELECT する
 * （cookieCache 未使用のため毎回DBに当たる）。layout の <Header /> と各 page.tsx が
 * それぞれ呼ぶと、1リクエストで同じ問い合わせが2回走っていた。
 *
 * React の cache() は同一リクエストのレンダリング中だけ結果を使い回すので、
 * 何箇所から呼んでもDBアクセスは1回で済む。引数を取らない形にしているのは、
 * メモ化のキーを安定させるため（headers() はこの中に閉じ込める）。
 *
 * API Route は Server Component と同じレンダリングパスに乗らないため対象外。
 */
export const getCurrentSession = cache(async () =>
  measured("auth.getSession", async () =>
    auth.api.getSession({ headers: await headers() })
  )
);
