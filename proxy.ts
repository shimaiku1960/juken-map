import { getSessionCookie } from "better-auth/cookies";
import { NextResponse, type NextRequest } from "next/server";

export const proxy = async (request: NextRequest) => {
  const sessionCookie = getSessionCookie(request);
  const { pathname } = request.nextUrl;

  // 誰でも見れるページ（トップ "/" はアカウント持ちのみなので含めない）
  const publicPaths = [
    "/login",
    "/signup",
    "/forgot-password",
    "/reset-password",
    "/blog",
  ];
  // 動的ルート（/articles/xxx）は前方一致で公開判定
  const publicPrefixes = ["/articles"];
  const isPublic =
    publicPaths.includes(pathname) ||
    publicPrefixes.some((prefix) => pathname.startsWith(prefix));

  // 未ログインで API を叩いたら 401
  if (!sessionCookie && pathname.startsWith("/api/")) {
    // Better Auth 自身のエンドポイントは除外
    if (!pathname.startsWith("/api/auth/")) {
      return NextResponse.json({ error: "未認証" }, { status: 401 });
    }
  }

  // 未ログインで保護ページに来たら /login へ
  if (!sessionCookie && !isPublic && !pathname.startsWith("/api/")) {
    const url = request.nextUrl.clone();
    url.pathname = "/login";
    return NextResponse.redirect(url);
  }

  return NextResponse.next();
};

export const config = {
  matcher: [
    "/((?!_next/static|_next/image|favicon.ico|manifest.webmanifest|.*\\.(?:svg|png|jpg|jpeg|gif|webp)$).*)",
  ],
};
