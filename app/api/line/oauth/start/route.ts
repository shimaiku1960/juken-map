import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { createLineOAuthValues, lineLoginAuthorizationUrl } from "@/lib/lineLogin";
import { SITE_URL } from "@/lib/site";

const CALLBACK_PATH = "/api/line/oauth/callback";

export async function GET(request: Request) {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    const origin = process.env.NODE_ENV === "production" ? SITE_URL : new URL(request.url).origin;
    return NextResponse.redirect(new URL("/login?callbackURL=%2Fprofile%23line-connection", origin));
  }

  try {
    const values = createLineOAuthValues();
    const origin = process.env.NODE_ENV === "production" ? SITE_URL : new URL(request.url).origin;
    const redirectUri = `${origin}${CALLBACK_PATH}`;
    await prisma.$transaction([
      prisma.lineOAuthAttempt.deleteMany({ where: { userId: session.user.id } }),
      prisma.lineOAuthAttempt.create({
        data: {
          state: values.state,
          nonce: values.nonce,
          codeVerifier: values.codeVerifier,
          redirectUri,
          userId: session.user.id,
          expiresAt: new Date(Date.now() + 10 * 60 * 1000),
        },
      }),
    ]);
    return NextResponse.redirect(lineLoginAuthorizationUrl({ ...values, redirectUri }));
  } catch (error) {
    console.error("[line-oauth] Failed to start LINE Login.", error);
    const origin = process.env.NODE_ENV === "production" ? SITE_URL : new URL(request.url).origin;
    return NextResponse.redirect(new URL("/profile?line=unavailable#line-connection", origin));
  }
}
