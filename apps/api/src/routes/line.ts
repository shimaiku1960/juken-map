import { randomBytes } from "node:crypto";
import type { FastifyInstance } from "fastify";
import { z } from "zod";
import {
  issueLineLinkToken,
  lineAccountLinkUrl,
  pushLineText,
  replyLineText,
  verifyLineSignature,
} from "@/api/infra/line";
import {
  createLineOAuthValues,
  exchangeLineLoginCode,
  getLineFriendshipStatus,
  lineLoginAuthorizationUrl,
  verifyLineIdToken,
} from "@/api/infra/lineLogin";
import { findLineConnection } from "@/api/services/notification-service";
import { SITE_URL } from "@/shared/site";
import {
  completeAccountLinkByNonce,
  disconnectLine,
  discardOAuthAttempt,
  findConnectionByLineUserId,
  findOAuthAttempt,
  issueLinkNonce,
  linkVerifiedLineUser,
  startOAuthAttempt,
} from "@/api/services/line-connection-service";
import { denyDemoWrite, getSession, requireSession } from "../context.ts";

// 分離前は「リダイレクト先の画面」と「API」が同じオリジンだったので
// new URL(request.url).origin で足りていた。分離後は画面が別プロセスになるため、
// 戻り先はフロントのオリジンを明示する。本番は nginx で同一オリジンなので SITE_URL。
function webOrigin() {
  return process.env.WEB_ORIGIN ?? SITE_URL;
}

const NOTIFICATION_SETTINGS_PATH = "/profile#notification-settings";
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
  const connection = await findConnectionByLineUserId(lineUserId);
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

  const result = await completeAccountLinkByNonce(nonce, lineUserId);

  if (result.status === "expired") {
    if (event.replyToken) {
      await replyLineText(
        event.replyToken,
        "連携リンクの期限が切れました。「連携」と送って、もう一度お試しください。"
      );
    }
    return;
  }

  const linked = result.status === "linked";

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
  // LINE のメッセージ本文が案内する導線。画面を持たず、ログイン状態で行き先を変えるだけ。
  // Next.js では app/line/settings/route.ts が同じことをしていた。SPA 側のルートに
  // しないのは、描画が要らずクライアント判定だと一瞬ちらつくため。
  app.get("/line/settings", async (request, reply) => {
    const session = await getSession(request);
    const origin = webOrigin();

    if (session) {
      return reply.redirect(`${origin}${NOTIFICATION_SETTINGS_PATH}`);
    }

    const loginUrl = new URL("/login", origin);
    loginUrl.searchParams.set("callbackURL", NOTIFICATION_SETTINGS_PATH);
    return reply.redirect(loginUrl.toString());
  });

  app.post("/api/line/account-link", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    const result = accountLinkSchema.safeParse(request.body);
    if (!result.success) {
      return reply.code(400).send({ error: "連携情報が正しくありません" });
    }

    const nonce = randomBytes(32).toString("base64url");
    await issueLinkNonce(session.user.id, nonce);

    const redirectUrl = new URL("https://access.line.me/dialog/bot/accountLink");
    redirectUrl.searchParams.set("linkToken", result.data.linkToken);
    redirectUrl.searchParams.set("nonce", nonce);
    return { redirectUrl: redirectUrl.toString() };
  });

  // Next.js では Server Component が findLineConnection を直接呼んでいたため
  // GET が無かった。SPA のプロフィール画面が連携状態を知る必要があるので新設する。
  app.get("/api/line/connection", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;

    const connection = await findLineConnection(session.user.id);
    return { connected: Boolean(connection) };
  });

  app.delete("/api/line/connection", async (request, reply) => {
    const session = await requireSession(request, reply);
    if (!session) return;
    if (denyDemoWrite(session, reply)) return;

    await disconnectLine(session.user.id);
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
      await startOAuthAttempt({
        userId: session.user.id,
        state: values.state,
        nonce: values.nonce,
        codeVerifier: values.codeVerifier,
        redirectUri,
      });
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

      const attempt = await findOAuthAttempt(state);
      if (
        !attempt ||
        attempt.userId !== session.user.id ||
        attempt.expiresAt <= new Date()
      ) {
        if (attempt) await discardOAuthAttempt(state);
        return profileRedirect("expired");
      }

      await discardOAuthAttempt(state);
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

        const linked = await linkVerifiedLineUser(session.user.id, identity.sub);
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
