import { randomBytes } from "node:crypto";
import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

const inputSchema = z.object({ linkToken: z.string().min(1).max(255) });

export async function POST(request: Request) {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) return NextResponse.json({ error: "未認証" }, { status: 401 });

  const result = inputSchema.safeParse(await request.json());
  if (!result.success) {
    return NextResponse.json({ error: "連携情報が正しくありません" }, { status: 400 });
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
  return NextResponse.json({ redirectUrl: redirectUrl.toString() });
}
