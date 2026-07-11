import { NextResponse } from "next/server";

// デモアカウント（面接官向け・閲覧専用）
export const DEMO_EMAIL = "demo@juken-map.com";

// デモユーザーの編集系リクエストなら 403 を返す。そうでなければ null。
// 各 API ルートで「if (!session) 401」の直後に呼ぶ想定。
export function demoReadOnlyGuard(
  session: { user: { email: string } } | null
): NextResponse | null {
  if (session?.user.email === DEMO_EMAIL) {
    return NextResponse.json(
      { error: "デモアカウントは閲覧専用です" },
      { status: 403 }
    );
  }
  return null;
}