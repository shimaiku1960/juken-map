// LPのスクリーンショット・動画撮影専用ユーザーを、ローカルDBへ再現可能な状態で投入する。
// 共有デモユーザーとは分離し、タイマー開始→実績保存まで実際に操作できるようにする。
// 実行: npm run capture:seed
import { PrismaClient } from "../app/generated/prisma/client";
import { PrismaMariaDb } from "@prisma/adapter-mariadb";
import { hashPassword } from "better-auth/crypto";
import { ymdAfterDays } from "../lib/date";

const CAPTURE_EMAIL = "capture@juken-map.com";
const CAPTURE_PASSWORD = "capturepass1234";

const databaseUrl = process.env.DATABASE_URL;
if (!databaseUrl) {
  throw new Error("DATABASE_URL が設定されていません");
}

const databaseHost = new URL(databaseUrl).hostname;
if (!["localhost", "127.0.0.1", "::1"].includes(databaseHost)) {
  throw new Error(
    `撮影用seedはローカルDB専用です（現在のホスト: ${databaseHost}）`
  );
}

const prisma = new PrismaClient({
  adapter: new PrismaMariaDb(databaseUrl),
});

async function upsertCredential(userId: string) {
  const passwordHash = await hashPassword(CAPTURE_PASSWORD);
  const existingCredential = await prisma.account.findFirst({
    where: { userId, providerId: "credential" },
  });

  if (existingCredential) {
    await prisma.account.update({
      where: { id: existingCredential.id },
      data: { password: passwordHash },
    });
    return;
  }

  await prisma.account.create({
    data: {
      userId,
      accountId: userId,
      providerId: "credential",
      password: passwordHash,
    },
  });
}

async function main() {
  const captureUser = await prisma.user.upsert({
    where: { email: CAPTURE_EMAIL },
    update: {
      name: "撮影用ユーザー",
      nickname: "受験マップ太郎",
      emailVerified: true,
    },
    create: {
      email: CAPTURE_EMAIL,
      name: "撮影用ユーザー",
      nickname: "受験マップ太郎",
      emailVerified: true,
    },
  });

  await upsertCredential(captureUser.id);

  // 撮影を毎回同じ初期状態から始めるため、撮影ユーザーの可変データだけをリセットする。
  await prisma.session.deleteMany({ where: { userId: captureUser.id } });
  await prisma.studyLog.deleteMany({ where: { userId: captureUser.id } });
  await prisma.studyPlan.deleteMany({ where: { userId: captureUser.id } });
  await prisma.finalGoal.deleteMany({ where: { userId: captureUser.id } });

  // 受験日程は第一志望だけだと1行で寂しいため、LPの撮影用に併願校まで揃える。
  // 受験日はマスター（Faculty.examDate）が持つので、ここでは大学・学部名だけ指定する。
  const goalSpecs: {
    university: string;
    faculty: string;
    isFirstChoice: boolean;
    note: string;
  }[] = [
    {
      university: "早稲田大学",
      faculty: "政治経済学部",
      isFirstChoice: true,
      note: "第一志望。英語と数学を重点的に取り組む。",
    },
    {
      university: "慶應義塾大学",
      faculty: "経済学部",
      isFirstChoice: false,
      note: "併願校。小論文の対策を別途進める。",
    },
    {
      university: "明治大学",
      faculty: "政治経済学部",
      isFirstChoice: false,
      note: "併願校。英語の配点が高い。",
    },
  ];

  for (const spec of goalSpecs) {
    const university = await prisma.university.findUnique({
      where: { name: spec.university },
    });
    if (!university) {
      throw new Error(
        `${spec.university} が見つかりません。先に通常のseedを実行してください`
      );
    }

    const faculty = await prisma.faculty.findFirst({
      where: { universityId: university.id, name: spec.faculty },
    });
    if (!faculty) {
      throw new Error(
        `${spec.university} ${spec.faculty} が見つかりません。先に通常のseedを実行してください`
      );
    }

    await prisma.finalGoal.create({
      data: {
        userId: captureUser.id,
        facultyId: faculty.id,
        isFirstChoice: spec.isFirstChoice,
        // decided かつ isFirstChoice=false が「併願校」として表示される。
        status: "decided",
        note: spec.note,
      },
    });
  }

  // 動画開始時点は今日1時間45分。英語30分を保存すると、LPの訴求値と同じ
  // 「今日2時間15分・英語5時間10分・8日連続」になる。
  const studyLogs: {
    offset: number;
    subject: "english" | "math" | "japanese";
    minutes: number;
  }[] = [
    { offset: -7, subject: "english", minutes: 40 },
    { offset: -6, subject: "english", minutes: 60 },
    { offset: -6, subject: "math", minutes: 30 },
    { offset: -5, subject: "english", minutes: 50 },
    { offset: -5, subject: "japanese", minutes: 30 },
    { offset: -4, subject: "math", minutes: 60 },
    { offset: -3, subject: "english", minutes: 70 },
    { offset: -3, subject: "japanese", minutes: 30 },
    { offset: -2, subject: "math", minutes: 60 },
    { offset: -1, subject: "english", minutes: 60 },
    { offset: -1, subject: "math", minutes: 60 },
    { offset: -1, subject: "japanese", minutes: 45 },
    { offset: 0, subject: "math", minutes: 60 },
    { offset: 0, subject: "japanese", minutes: 45 },
  ];

  await prisma.studyLog.createMany({
    data: studyLogs.map((log) => ({
      userId: captureUser.id,
      date: new Date(ymdAfterDays(log.offset)),
      subject: log.subject,
      minutes: log.minutes,
    })),
  });

  await prisma.studyPlan.createMany({
    data: [
      {
        userId: captureUser.id,
        date: new Date(ymdAfterDays(0)),
        content: "英語長文 1題（早稲田過去問）",
        subject: "english",
        done: false,
      },
      {
        userId: captureUser.id,
        date: new Date(ymdAfterDays(0)),
        content: "数学ⅠA 二次関数 演習",
        subject: "math",
        done: true,
      },
      {
        userId: captureUser.id,
        date: new Date(ymdAfterDays(0)),
        content: "現代文 評論 読解1題",
        subject: "japanese",
        done: true,
      },
      {
        userId: captureUser.id,
        date: new Date(ymdAfterDays(1)),
        content: "日本史 近現代 通史",
        subject: "social",
        done: false,
      },
      {
        userId: captureUser.id,
        date: new Date(ymdAfterDays(3)),
        content: "英文法 Vintage 仮定法",
        subject: "english",
        done: false,
      },
    ],
  });

  console.log("LP撮影用データを初期状態へ戻しました");
  console.log(`メールアドレス: ${CAPTURE_EMAIL}`);
  console.log(`パスワード: ${CAPTURE_PASSWORD}`);
  console.log("動画では英語長文を30分として保存してください");
}

main()
  .then(() => prisma.$disconnect())
  .catch(async (error) => {
    console.error(error);
    await prisma.$disconnect();
    process.exit(1);
  });
