// E2E 用の通常ユーザーを投入する tsx スクリプト（Playwright の globalSetup から呼ぶ）。
// Prisma クライアントは ESM（import.meta）なので、Playwright 本体ではなく
// tsx の子プロセスで実行して読み込む。
import { PrismaClient } from "../app/generated/prisma/client";
import { PrismaMariaDb } from "@prisma/adapter-mariadb";
import { hashPassword } from "better-auth/crypto";
import { E2E_EMAIL, E2E_PASSWORD } from "../e2e/credentials";

const prisma = new PrismaClient({
  adapter: new PrismaMariaDb(process.env.DATABASE_URL!),
});

async function main() {
  const user = await prisma.user.upsert({
    where: { email: E2E_EMAIL },
    // メール確認必須化後も E2E ログインが通るよう、既存行にも確認済みを保証する
    update: { emailVerified: true },
    create: {
      email: E2E_EMAIL,
      name: "E2Eユーザー",
      nickname: "E2E",
      emailVerified: true,
    },
  });

  // Better Auth の credential アカウントを純正ハッシュで用意（再実行時は更新）
  const passwordHash = await hashPassword(E2E_PASSWORD);
  const existing = await prisma.account.findFirst({
    where: { userId: user.id, providerId: "credential" },
  });
  if (existing) {
    await prisma.account.update({
      where: { id: existing.id },
      data: { password: passwordHash },
    });
  } else {
    await prisma.account.create({
      data: {
        userId: user.id,
        accountId: user.id,
        providerId: "credential",
        password: passwordHash,
      },
    });
  }

  await prisma.textbook.upsert({
    where: {
      userId_name: { userId: user.id, name: "E2E英語教材" },
    },
    update: { subject: "english", rangeUnit: "page" },
    create: {
      userId: user.id,
      name: "E2E英語教材",
      subject: "english",
      rangeUnit: "page",
    },
  });
  console.log(`E2Eユーザーを投入: ${E2E_EMAIL}`);
}

main()
  .then(() => prisma.$disconnect())
  .catch(async (e) => {
    console.error(e);
    await prisma.$disconnect();
    process.exit(1);
  });
