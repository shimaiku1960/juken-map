import prisma from "../lib/prisma";
import { hashSupportLineCode } from "../lib/support-line";

async function main() {
  const code = process.argv[2]?.trim();
  const lineDisplayName = process.argv.slice(3).join(" ").trim() || null;

  if (!code) {
    throw new Error(
      "Usage: npm run support:line-link -- CODE [LINE表示名]"
    );
  }

  const codeHash = hashSupportLineCode(code);
  const connection = await prisma.supportLineConnection.findUnique({
    where: { codeHash },
    select: {
      id: true,
      codeExpiresAt: true,
      linkedAt: true,
      user: { select: { email: true } },
    },
  });

  if (!connection || connection.linkedAt) {
    throw new Error("照合コードが無効または使用済みです");
  }
  if (!connection.codeExpiresAt || connection.codeExpiresAt <= new Date()) {
    throw new Error("照合コードの有効期限が切れています。利用者に再発行を依頼してください");
  }

  const result = await prisma.supportLineConnection.updateMany({
    where: {
      id: connection.id,
      codeHash,
      linkedAt: null,
      codeExpiresAt: { gt: new Date() },
    },
    data: {
      linkedAt: new Date(),
      lineDisplayName,
      codeHash: null,
      codeExpiresAt: null,
    },
  });

  if (result.count !== 1) {
    throw new Error("照合コードの状態が変わりました。もう一度確認してください");
  }

  console.log(`LINE連携完了: ${connection.user.email}`);
  if (lineDisplayName) console.log(`LINE表示名: ${lineDisplayName}`);
}

main()
  .catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  })
  .finally(async () => {
    await prisma.$disconnect();
  });
