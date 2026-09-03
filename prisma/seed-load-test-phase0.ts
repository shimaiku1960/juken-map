import { PrismaClient } from "../app/generated/prisma/client";
import { PrismaMariaDb } from "@prisma/adapter-mariadb";
import { hashPassword } from "better-auth/crypto";

const EMAIL = "loadtest-phase0@juken-map.invalid";
const PASSWORD = "loadtest-phase0-password";
const PLAN_MARKER = "LOADTEST_PHASE0_CONCURRENT_COMPLETE";

const prisma = new PrismaClient({
  adapter: new PrismaMariaDb(process.env.DATABASE_URL!),
});

async function main() {
  const user = await prisma.user.upsert({
    where: { email: EMAIL },
    update: { emailVerified: true, firstStudyLogAt: null },
    create: {
      email: EMAIL,
      name: "Load Test Phase 0",
      nickname: "Load Test",
      emailVerified: true,
    },
  });

  const passwordHash = await hashPassword(PASSWORD);
  const account = await prisma.account.findFirst({
    where: { userId: user.id, providerId: "credential" },
  });
  if (account) {
    await prisma.account.update({
      where: { id: account.id },
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

  await prisma.studyLog.deleteMany({
    where: { userId: user.id, studyPlan: { content: PLAN_MARKER } },
  });
  await prisma.studyPlan.deleteMany({
    where: { userId: user.id, content: PLAN_MARKER },
  });
  await prisma.session.deleteMany({ where: { userId: user.id } });

  const plan = await prisma.studyPlan.create({
    data: {
      userId: user.id,
      date: new Date(),
      content: PLAN_MARKER,
      subject: "english",
    },
  });

  process.stdout.write(
    JSON.stringify({ email: EMAIL, password: PASSWORD, planId: plan.id })
  );
}

main()
  .then(() => prisma.$disconnect())
  .catch(async (error) => {
    console.error(error);
    await prisma.$disconnect();
    process.exit(1);
  });
