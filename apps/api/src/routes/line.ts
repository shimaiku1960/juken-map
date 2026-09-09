import { randomBytes } from "node:crypto";
import type { FastifyInstance } from "fastify";
import { z } from "zod";
import { prisma } from "@/backend/infra/prisma";
import {
  issueLineLinkToken,
  lineAccountLinkUrl,
  pushLineText,
  replyLineText,
  verifyLineSignature,
} from "@/backend/infra/line";
import {
  createLineOAuthValues,
  exchangeLineLoginCode,
  getLineFriendshipStatus,
  lineLoginAuthorizationUrl,
  verifyLineIdToken,
} from "@/backend/infra/lineLogin";
import { SITE_URL } from "@/shared/site";
import { denyDemoWrite, getSession, requireSession } from "../context.ts";

// 分離前は「リダイレクト先の画面」と「API」が同じオリジンだったので
// new URL(request.url).origin で足りていた。分離後は画面が別プロセスになるため、
// 戻り先はフロントのオリジンを明示する。本番は nginx で同一オリジンなので SITE_URL。
function webOrigin() {
  return process.env.WEB_ORIGIN ?? SITE_URL;
}

const CALLBACK_PATH = "/api/line/oauth/callback";
const LINE_CONNECTION_COMPLETED_MESSAGE = [
  "受験マップとのLINE連携が完了しました！",
  "",
  "朝・夜の通知は、受験マップのプロフィールから設定できます。",
  new URL("/line/settings", SITE_URL).toString(),
].join("\n");
const LINE_CONFIRMATION_TIMEOUT_MS = 3_000;

const accountLinkSchema = z.object({ linkToken: z.string().min(1).max(255) });

type LineEvent = {
  type: string;
  replyToken?: string;
  source?: { type?: string; userId?: string };
  message?: { type?: string; text?: string };
  link?: { result?: string; nonce?: string };
};

async function sendLinkGuide(event: LineEvent) {
  const lineUserId = event.source?.userId;
  if (!lineUserId || !event.replyToken) return;
  const connection = await prisma.lineConnection.findUnique({
    where: { lineUserId },
    select: { id: true },
  });
  if (connection) {
    await replyLineText(
      event.replyToken,
      `受験マップとはすでに連携済みです。\n通知設定を確認する → ${SITE_URL}/line/settings`
    );
    return;
  }
  const linkToken = await issueLineLinkToken(lineUserId);
  await replyLineText(
    event.replyToken,
    `受験マップとLINEを連携します。次のリンクを10分以内に開いてログインしてください。\n${lineAccountLinkUrl(linkToken)}`
  );
}

async function completeAccountLink(event: LineEvent) {
  if (event.link?.result !== "ok") return;
  const nonce = event.link.nonce;
  const lineUserId = event.source?.userId;
  if (!nonce || !lineUserId) return;

  const linkNonce = await prisma.lineLinkNonce.findUnique({ where: { nonce } });
  if (!linkNonce || linkNonce.expiresAt <= new Date()) {
    if (event.replyToken) {
      await replyLineText(
        event.replyToken,
        "連携リンクの期限が切れました。「連携」と送って、もう一度お試しください。"
      );
    }
    return;
  }

  const linked = await prisma.$transaction(async (tx) => {
    const current = await tx.lineConnection.findUnique({ where: { lineUserId } });
    if (current && current.userId !== linkNonce.userId) {
      await tx.lineLinkNonce.delete({ where: { nonce } });
      return false;
    }
    await tx.lineConnection.upsert({
      where: { userId: linkNonce.userId },
      create: { userId: linkNonce.userId, lineUserId },
      update: { lineUserId, linkedAt: new Date() },
    });
    await tx.lineLinkNonce.delete({ where: { nonce } });
    return true;
  });

  if (event.replyToken) {
    await replyLineText(
      event.replyToken,
      linked
        ? `受験マップとの連携が完了しました。\n通知設定を続ける → ${SITE_URL}/line/settings`
        : "このLINEは別の受験マップアカウントに連携済みです。以前のアカウントでLINE連携を解除してから、もう一度お試しください。"
    );
  }
}

export function registerLineRoutes(app: FastifyInstance) {
  app.post("/api/line/account-link", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    const result = accountLinkSchema.safeParse(request.body);
    if (!result.success) {
      return reply.code(400).send({ error: "連携情報が正しくありません" });
    }

    const nonce = randomBytes(32).toString("base64url");
    await prisma.$transaction([
      prisma.lineLinkNonce.deleteMany({ where: { userId: session.user.id } }),
      prisma.lineLinkNonce.create({
        data: {
          nonce,
          userId: session.user.id,
          expiresAt: new Date(Date.now() + 10 * 60 * 1000),
        },
      }),
    ]);

    const redirectUrl = new URL("https://access.line.me/dialog/bot/accountLink");
    redirectUrl.searchParams.set("linkToken", result.data.linkToken);
    redirectUrl.searchParams.set("nonce", nonce);
    return { redirectUrl: redirectUrl.toString() };
  });

  app.delete("/api/line/connection", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    await prisma.$transaction([
      prisma.notificationPreference.updateMany({
        where: { userId: session.user.id },
        data: { lineMorningEnabled: false, lineEveningEnabled: false },
      }),
      prisma.lineConnection.deleteMany({ where: { userId: session.user.id } }),
      prisma.lineLinkNonce.deleteMany({ where: { userId: session.user.id } }),
      prisma.lineOAuthAttempt.deleteMany({ where: { userId: session.user.id } }),
    ]);
    return { connected: false };
  });

  app.get("/api/line/oauth/start", async (request, reply) => {
    const session = await getSession(request);
    if (!session) {
      return reply.redirect(
        `${webOrigin()}/login?callbackURL=%2Fprofile%23line-connection`
      );
    }

    try {
      const values = createLineOAuthValues();
      const redirectUri = `${webOrigin()}${CALLBACK_PATH}`;
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
      return reply.redirect(
        lineLoginAuthorizationUrl({ ...values, redirectUri }).toString()
      );
    } catch (error) {
      console.error("[line-oauth] Failed to start LINE Login.", error);
      return reply.redirect(`${webOrigin()}/profile?line=unavailable#line-connection`);
    }
  });

  app.get<{ Querystring: { state?: string; code?: string; error?: string } }>(
    "/api/line/oauth/callback",
    async (request, reply) => {
      const profileRedirect = (result: string) =>
        reply.redirect(`${webOrigin()}/profile?line=${result}#line-connection`);

      const { state, code, error: oauthError } = request.query;
      if (oauthError !== undefined) return profileRedirect("cancelled");
      if (!state || !code) return profileRedirect("invalid");

      const session = await getSession(request);
      if (!session) {
        const callbackURL = `${CALLBACK_PATH}?${new URLSearchParams({ state, code })}`;
        return reply.redirect(
          `${webOrigin()}/login?callbackURL=${encodeURIComponent(callbackURL)}`
        );
      }

      const attempt = await prisma.lineOAuthAttempt.findUnique({ where: { state } });
      if (
        !attempt ||
        attempt.userId !== session.user.id ||
        attempt.expiresAt <= new Date()
      ) {
        if (attempt) await prisma.lineOAuthAttempt.deleteMany({ where: { state } });
        return profileRedirect("expired");
      }

      await prisma.lineOAuthAttempt.delete({ where: { state } });
      try {
        const tokens = await exchangeLineLoginCode({
          code,
          codeVerifier: attempt.codeVerifier,
          redirectUri: attempt.redirectUri,
        });
        const [identity, friendship] = await Promise.all([
          verifyLineIdToken(tokens.id_token, attempt.nonce),
          getLineFriendshipStatus(tokens.access_token),
        ]);
        if (!identity.sub || identity.nonce !== attempt.nonce) {
          return profileRedirect("invalid");
        }
        if (!friendship.friendFlag) return profileRedirect("friend-required");

        const linked = await prisma.$transaction(async (tx) => {
          const current = await tx.lineConnection.findUnique({
            where: { lineUserId: identity.sub },
          });
          if (current && current.userId !== session.user.id) return false;
          await tx.lineConnection.upsert({
            where: { userId: session.user.id },
            create: { userId: session.user.id, lineUserId: identity.sub },
            update: { lineUserId: identity.sub, linkedAt: new Date() },
          });
          return true;
        });
        if (!linked) return profileRedirect("already-used");

        const controller = new AbortController();
        const timeout = setTimeout(
          () => controller.abort(),
          LINE_CONFIRMATION_TIMEOUT_MS
        );
        try {
          await pushLineText(
            identity.sub,
            LINE_CONNECTION_COMPLETED_MESSAGE,
            controller.signal
          );
        } catch (error) {
          console.error(
            "[line-oauth] LINE connection completed, but confirmation message failed.",
            error
          );
        } finally {
          clearTimeout(timeout);
        }
        return profileRedirect("connected");
      } catch (error) {
        console.error("[line-oauth] Failed to complete LINE Login.", error);
        return profileRedirect("failed");
      }
    }
  );

  // Webhook は署名検証に「パース前の生ボディ」が要る。
  // Fastify のコンテンツタイプパーサはプラグインのスコープに閉じるので、
  // この経路だけを別プラグインにして生文字列で受け取る。
  app.register(async (scope) => {
    // アプリ全体には JSON を解析するパーサが登録済みなので、このスコープでは
    // 一度外してから「生文字列のまま渡す」パーサを入れる。
    // 同じ content-type を二重登録すると FST_ERR_CTP_ALREADY_PRESENT で起動に失敗する。
    scope.removeContentTypeParser("application/json");
    scope.addContentTypeParser(
      "application/json",
      { parseAs: "string" },
      (_req, body, done) => done(null, body)
    );

    scope.post("/api/line/webhook", async (request, reply) => {
      const body = request.body as string;
      const signature = request.headers["x-line-signature"];
      if (
        !verifyLineSignature(body, typeof signature === "string" ? signature : null)
      ) {
        return reply.code(401).send({ error: "Invalid signature" });
      }

      const payload = JSON.parse(body) as { events?: LineEvent[] };
      for (const event of payload.events ?? []) {
        try {
          if (event.type === "accountLink") {
            await completeAccountLink(event);
          } else if (
            event.type === "follow" ||
            (event.type === "message" &&
              event.message?.type === "text" &&
              event.message.text?.trim() === "連携")
          ) {
            await sendLinkGuide(event);
          } else if (event.type === "message" && event.replyToken) {
            await replyLineText(
              event.replyToken,
              "受験マップとつなぐには「連携」と送ってください。"
            );
          }
        } catch (error) {
          console.error(`[line-webhook] ${event.type} processing failed.`, error);
        }
      }

      return { ok: true };
    });
  });
}
