// デモユーザー「だけ」を投入する一回きりスクリプト（本番/ローカル共通）。
// 大学マスターには一切触れず、既存の Faculty を参照して志望校・学習予定を作る。
// 本番へは EC2 を踏み台にした SSH トンネル経由で DATABASE_URL を向けて実行する想定。
//   例) DATABASE_URL="mysql://user:pass@127.0.0.1:3307/db" npx tsx prisma/seed-demo.ts
import { PrismaClient } from "../app/generated/prisma/client";
import { PrismaMariaDb } from "@prisma/adapter-mariadb";
import { hashPassword } from "better-auth/crypto";
import { ymdAfterDays } from "../lib/date";

const prisma = new PrismaClient({
  adapter: new PrismaMariaDb(process.env.DATABASE_URL!),
});

const DEMO_EMAIL = "demo@juken-map.com";
const DEMO_PASSWORD = "demodemo1234";

async function main() {
  // 1. デモユーザー本体（面接官がワンクリックで体験する共有アカウント）
  const demoUser = await prisma.user.upsert({
    where: { email: DEMO_EMAIL },
    update: {},
    create: {
      email: DEMO_EMAIL,
      name: "デモユーザー",
      nickname: "デモ太郎",
      emailVerified: true,
    },
  });

  // 2. Better Auth の認証情報（credential アカウント）を純正ハッシュで用意
  const passwordHash = await hashPassword(DEMO_PASSWORD);
  const existingCredential = await prisma.account.findFirst({
    where: { userId: demoUser.id, providerId: "credential" },
  });
  if (existingCredential) {
    await prisma.account.update({
      where: { id: existingCredential.id },
      data: { password: passwordHash },
    });
  } else {
    await prisma.account.create({
      data: {
        userId: demoUser.id,
        accountId: demoUser.id,
        providerId: "credential",
        password: passwordHash,
      },
    });
  }
  console.log(`デモユーザーを投入: ${DEMO_EMAIL}`);

  // 3. 志望校（FinalGoal）サンプル。既存の大学＋学部を名前で引いて紐づける。
  const demoGoals: { university: string; faculty: string; isFirstChoice: boolean; note?: string }[] = [
    { university: "早稲田大学", faculty: "政治経済学部", isFirstChoice: true, note: "第一志望。英語と数学を重点的に。" },
    { university: "慶應義塾大学", faculty: "経済学部", isFirstChoice: false, note: "小論文対策が必要。" },
    { university: "明治大学", faculty: "政治経済学部", isFirstChoice: false },
    { university: "中央大学", faculty: "経済学部", isFirstChoice: false, note: "併願の安全校。" },
  ];

  for (const g of demoGoals) {
    const university = await prisma.university.findUniqueOrThrow({
      where: { name: g.university },
    });
    const faculty = await prisma.faculty.findFirstOrThrow({
      where: { name: g.faculty, universityId: university.id },
    });

    await prisma.finalGoal.upsert({
      where: { userId_facultyId: { userId: demoUser.id, facultyId: faculty.id } },
      update: { isFirstChoice: g.isFirstChoice, note: g.note ?? null },
      create: {
        userId: demoUser.id,
        facultyId: faculty.id,
        isFirstChoice: g.isFirstChoice,
        note: g.note ?? null,
      },
    });
  }
  console.log(`デモの志望校を投入: ${demoGoals.length}件`);

  // 4. 学習予定（StudyPlan）サンプル。今日基準の相対日付。過去=完了、今日/未来=未完了。
  //    再実行で重複しないよう、デモの既存予定だけ消してから入れ直す（デモのリセット）。
  await prisma.studyPlan.deleteMany({ where: { userId: demoUser.id } });

  const demoPlans: { offset: number; content: string; subject: string; done: boolean }[] = [
    { offset: -3, content: "英単語 ターゲット1900（前半）", subject: "english", done: true },
    { offset: -3, content: "数学ⅠA 二次関数 演習", subject: "math", done: true },
    { offset: -1, content: "現代文 評論 読解1題", subject: "japanese", done: true },
    { offset: 0, content: "英語長文 1題（早稲田過去問）", subject: "english", done: false },
    { offset: 0, content: "日本史 近現代 通史", subject: "social", done: false },
    { offset: 1, content: "数学ⅡB ベクトル", subject: "math", done: false },
    { offset: 3, content: "英文法 Vintage 仮定法", subject: "english", done: false },
    { offset: 6, content: "古文 助動詞 暗記", subject: "japanese", done: false },
  ];

  await prisma.studyPlan.createMany({
    data: demoPlans.map((p) => ({
      userId: demoUser.id,
      date: new Date(ymdAfterDays(p.offset)),
      content: p.content,
      subject: p.subject,
      done: p.done,
    })),
  });
  console.log(`デモの学習予定を投入: ${demoPlans.length}件`);

  // 5. 学習実績（StudyLog）サンプル。今日基準の相対日付で直近2週間ぶん。
  //    ヒートマップ・ストリーク・科目別バーが「動いて見える」よう科目と時間を散らす。
  //    再実行で重複しないよう、デモの既存実績だけ消してから入れ直す（デモのリセット）。
  await prisma.studyLog.deleteMany({ where: { userId: demoUser.id } });

  const demoLogs: { offset: number; subject: string; minutes: number }[] = [
    { offset: -13, subject: "english", minutes: 60 },
    { offset: -12, subject: "math", minutes: 90 },
    { offset: -10, subject: "japanese", minutes: 45 },
    { offset: -9, subject: "english", minutes: 30 },
    { offset: -9, subject: "social", minutes: 40 },
    { offset: -8, subject: "math", minutes: 120 },
    // 直近7日は連続で記録（ストリーク7日）
    { offset: -6, subject: "english", minutes: 80 },
    { offset: -5, subject: "math", minutes: 60 },
    { offset: -5, subject: "japanese", minutes: 30 },
    { offset: -4, subject: "social", minutes: 50 },
    { offset: -3, subject: "english", minutes: 70 },
    { offset: -3, subject: "math", minutes: 40 },
    { offset: -2, subject: "japanese", minutes: 55 },
    { offset: -1, subject: "english", minutes: 90 },
    { offset: -1, subject: "science", minutes: 45 },
    { offset: 0, subject: "math", minutes: 60 },
    { offset: 0, subject: "english", minutes: 50 },
  ];

  await prisma.studyLog.createMany({
    data: demoLogs.map((l) => ({
      userId: demoUser.id,
      date: new Date(ymdAfterDays(l.offset)),
      minutes: l.minutes,
      subject: l.subject,
    })),
  });
  console.log(`デモの学習実績を投入: ${demoLogs.length}件`);
}

main()
  .then(() => prisma.$disconnect())
  .catch(async (e) => {
    console.error(e);
    await prisma.$disconnect();
    process.exit(1);
  });
