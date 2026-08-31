import { NextResponse } from "next/server";
import prisma from "@/lib/prisma";
import {
  issueLineLinkToken,
  lineAccountLinkUrl,
  replyLineText,
  verifyLineSignature,
} from "@/lib/line";
import { SITE_URL } from "@/lib/site";

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
      await replyLineText(event.replyToken, "連携リンクの期限が切れました。「連携」と送って、もう一度お試しください。");
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
        ? `受験マップとの連携が完了しました。\n通知設定を続ける → ${SITE_URL}/profile#notification-settings`
        : "このLINEは別の受験マップアカウントに連携済みです。以前のアカウントでLINE連携を解除してから、もう一度お試しください。"
    );
  }
}

export async function POST(request: Request) {
  const body = await request.text();
  if (!verifyLineSignature(body, request.headers.get("x-line-signature"))) {
    return NextResponse.json({ error: "Invalid signature" }, { status: 401 });
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
        await replyLineText(event.replyToken, "受験マップとつなぐには「連携」と送ってください。");
      }
    } catch (error) {
      console.error(`[line-webhook] ${event.type} processing failed.`, error);
    }
  }

  return NextResponse.json({ ok: true });
}
