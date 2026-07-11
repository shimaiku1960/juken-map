import { NextResponse } from "next/server";
import { DEMO_EMAIL } from "./demo-email";

// クライアントからも使えるよう定数は demo-email.ts に分離し、ここから再エクスポートする
export { DEMO_EMAIL };

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