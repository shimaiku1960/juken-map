import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { pushLineText } from "@/lib/line";
import prisma from "@/lib/prisma";
import { exchangeLineLoginCode, getLineFriendshipStatus, verifyLineIdToken } from "@/lib/lineLogin";
import { SITE_URL } from "@/lib/site";

function appOrigin(request: Request) {
  return process.env.NODE_ENV === "production" ? SITE_URL : new URL(request.url).origin;
}

function profileRedirect(request: Request, result: string) {
  return NextResponse.redirect(new URL(`/profile?line=${result}#line-connection`, appOrigin(request)));
}

const LINE_CONNECTION_COMPLETED_MESSAGE = [
  "受験マップとのLINE連携が完了しました！",
  "",
  "朝・夜の通知は、受験マップのプロフィールから設定できます。",
  new URL("/line/settings", SITE_URL).toString(),
].join("\n");
const LINE_CONFIRMATION_TIMEOUT_MS = 3_000;

export async function GET(request: Request) {
  const url = new URL(request.url);
  const state = url.searchParams.get("state");
  const code = url.searchParams.get("code");
  if (url.searchParams.has("error")) return profileRedirect(request, "cancelled");
  if (!state || !code) return profileRedirect(request, "invalid");

  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    const callbackURL = `${url.pathname}${url.search}`;
    return NextResponse.redirect(new URL(`/login?callbackURL=${encodeURIComponent(callbackURL)}`, appOrigin(request)));
  }

  const attempt = await prisma.lineOAuthAttempt.findUnique({ where: { state } });
  if (!attempt || attempt.userId !== session.user.id || attempt.expiresAt <= new Date()) {
    if (attempt) await prisma.lineOAuthAttempt.deleteMany({ where: { state } });
    return profileRedirect(request, "expired");
  }

  await prisma.lineOAuthAttempt.delete({ where: { state } });
  try {
    const tokens = await exchangeLineLoginCode({ code, codeVerifier: attempt.codeVerifier, redirectUri: attempt.redirectUri });
    const [identity, friendship] = await Promise.all([
      verifyLineIdToken(tokens.id_token, attempt.nonce),
      getLineFriendshipStatus(tokens.access_token),
    ]);
    if (!identity.sub || identity.nonce !== attempt.nonce) return profileRedirect(request, "invalid");
    if (!friendship.friendFlag) return profileRedirect(request, "friend-required");

    const linked = await prisma.$transaction(async (tx) => {
      const current = await tx.lineConnection.findUnique({ where: { lineUserId: identity.sub } });
      if (current && current.userId !== session.user.id) return false;
      await tx.lineConnection.upsert({
        where: { userId: session.user.id },
        create: { userId: session.user.id, lineUserId: identity.sub },
        update: { lineUserId: identity.sub, linkedAt: new Date() },
      });
      return true;
    });
    if (!linked) return profileRedirect(request, "already-used");

    const confirmationController = new AbortController();
    const confirmationTimeout = setTimeout(() => confirmationController.abort(), LINE_CONFIRMATION_TIMEOUT_MS);
    try {
      await pushLineText(identity.sub, LINE_CONNECTION_COMPLETED_MESSAGE, confirmationController.signal);
    } catch (error) {
      console.error("[line-oauth] LINE connection completed, but confirmation message failed.", error);
    } finally {
      clearTimeout(confirmationTimeout);
    }
    return profileRedirect(request, "connected");
  } catch (error) {
    console.error("[line-oauth] Failed to complete LINE Login.", error);
    return profileRedirect(request, "failed");
  }
}
