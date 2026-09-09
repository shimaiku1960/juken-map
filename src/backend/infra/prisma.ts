import { PrismaClient } from "@/app/generated/prisma/client";
import { PrismaMariaDb } from "@prisma/adapter-mariadb";

const globalForPrisma = globalThis as unknown as { prisma: PrismaClient };

const prisma =
  globalForPrisma.prisma ||
  new PrismaClient({
    adapter: new PrismaMariaDb(process.env.DATABASE_URL!),
    log: process.env.NODE_ENV === "development" ? ["query"] : undefined,
  });

if (process.env.NODE_ENV !== "production") globalForPrisma.prisma = prisma;

// 名前付きエクスポートを併記している。apps/api（"type": "module"）から読むと、
// ルートに "type": "module" が無いためこのファイルは CommonJS 扱いになり、
// default import が { default: ... } に包まれてしまう。名前付きなら影響を受けない。
// Next.js を削除して apps/api だけになったら default は消してよい。
export { prisma };
export default prisma;
