
import { readFileSync } from "fs";
import { PrismaClient } from "../app/generated/prisma/client";
import { PrismaMariaDb } from "@prisma/adapter-mariadb";
import { hashPassword } from "better-auth/crypto";
import { ymdAfterDays } from "../lib/date";

const prisma = new PrismaClient({
  adapter: new PrismaMariaDb( process.env.DATABASE_URL! ),
});

// 全国大学マスター（scripts/transform-universities.ts が生成）
type UniversityRow = { name: string; prefecture: string; type: string };
const universities: UniversityRow[] = JSON.parse(
  readFileSync("data/clean/universities.json", "utf-8")
);

// 大学ごとの学部データ（学部はここを手動で拡充していく）
// ⚠️  examDate は2026年度入試の実績ベースの暫定値。2027年度の正式日程が
//    各大学から発表されたら要更新（現時点では未発表のため暫定）
const facultyData: Record<
  string,
  { name: string; examDate: string; tags: string[] }[]
> = {
  早稲田大学: [
    { name: "政治経済学部", examDate: "2027-02-20", tags: ["法・政経系", "商・経営系"] },
    { name: "法学部", examDate: "2027-02-15", tags: ["法・政経系"] },
    { name: "商学部", examDate: "2027-02-21", tags: ["商・経営系"] },
    { name: "文学部", examDate: "2027-02-17", tags: ["文・文化系"] },
    { name: "文化構想学部", examDate: "2027-02-12", tags: ["文・文化系"] },
    { name: "基幹理工学部", examDate: "2027-02-16", tags: ["理工系", "情報系"] },
    { name: "創造理工学部", examDate: "2027-02-16", tags: ["理工系"] },
    { name: "先進理工学部", examDate: "2027-02-16", tags: ["理工系", "農・生命系"] },
  ],
  慶應義塾大学: [
    { name: "経済学部", examDate: "2027-02-13", tags: ["法・政経系", "商・経営系"] },
    { name: "法学部", examDate: "2027-02-16", tags: ["法・政経系"] },
    { name: "商学部", examDate: "2027-02-14", tags: ["商・経営系"] },
    { name: "文学部", examDate: "2027-02-15", tags: ["文・文化系"] },
    { name: "理工学部", examDate: "2027-02-12", tags: ["理工系", "情報系"] },
    { name: "薬学部", examDate: "2027-02-10", tags: ["医歯薬系"] },
    { name: "医学部", examDate: "2027-02-19", tags: ["医歯薬系"] },
    { name: "看護医療学部", examDate: "2027-02-11", tags: ["医歯薬系"] },
  ],
  明治大学: [
    { name: "法学部", examDate: "2027-02-14", tags: ["法・政経系"] },
    { name: "政治経済学部", examDate: "2027-02-11", tags: ["法・政経系", "商・経営系"] },
    { name: "商学部", examDate: "2027-02-16", tags: ["商・経営系"] },
    { name: "経営学部", examDate: "2027-02-10", tags: ["商・経営系"] },
    { name: "文学部", examDate: "2027-02-13", tags: ["文・文化系"] },
    { name: "国際日本学部", examDate: "2027-02-09", tags: ["文・文化系"] },
    { name: "情報コミュニケーション学部", examDate: "2027-02-08", tags: ["文・文化系", "情報系"] },
    { name: "理工学部", examDate: "2027-02-07", tags: ["理工系", "情報系"] },
    { name: "農学部", examDate: "2027-02-15", tags: ["農・生命系"] },
    { name: "総合数理学部", examDate: "2027-02-17", tags: ["理工系", "情報系"] },
  ],
  青山学院大学: [
    { name: "文学部", examDate: "2027-02-13", tags: ["文・文化系"] },
    { name: "教育人間科学部", examDate: "2027-02-13", tags: ["文・文化系"] },
    { name: "経済学部", examDate: "2027-02-19", tags: ["法・政経系", "商・経営系"] },
    { name: "法学部", examDate: "2027-02-18", tags: ["法・政経系"] },
    { name: "経営学部", examDate: "2027-02-15", tags: ["商・経営系"] },
    { name: "国際政治経済学部", examDate: "2027-02-17", tags: ["法・政経系", "商・経営系"] },
    { name: "総合文化政策学部", examDate: "2027-02-09", tags: ["文・文化系"] },
    { name: "理工学部", examDate: "2027-02-10", tags: ["理工系", "情報系"] },
    { name: "社会情報学部", examDate: "2027-02-09", tags: ["情報系", "文・文化系"] },
    { name: "地球社会共生学部", examDate: "2027-02-18", tags: ["文・文化系"] },
    { name: "コミュニティ人間科学部", examDate: "2027-02-11", tags: ["文・文化系"] },
  ],
  上智大学: [
    { name: "神学部", examDate: "2027-02-07", tags: ["文・文化系"] },
    { name: "文学部", examDate: "2027-02-07", tags: ["文・文化系"] },
    { name: "総合人間科学部", examDate: "2027-02-07", tags: ["文・文化系"] },
    { name: "法学部", examDate: "2027-02-09", tags: ["法・政経系"] },
    { name: "経済学部", examDate: "2027-02-09", tags: ["法・政経系", "商・経営系"] },
    { name: "外国語学部", examDate: "2027-02-10", tags: ["文・文化系"] },
    { name: "総合グローバル学部", examDate: "2027-02-10", tags: ["文・文化系"] },
    { name: "理工学部", examDate: "2027-02-11", tags: ["理工系", "情報系"] },
  ],
  中央大学: [
    { name: "法学部", examDate: "2027-02-12", tags: ["法・政経系"] },
    { name: "経済学部", examDate: "2027-02-14", tags: ["法・政経系", "商・経営系"] },
    { name: "商学部", examDate: "2027-02-11", tags: ["商・経営系"] },
    { name: "文学部", examDate: "2027-02-10", tags: ["文・文化系"] },
    { name: "総合政策学部", examDate: "2027-02-16", tags: ["法・政経系", "文・文化系"] },
    { name: "国際経営学部", examDate: "2027-02-10", tags: ["商・経営系"] },
    { name: "国際情報学部", examDate: "2027-02-11", tags: ["情報系"] },
    { name: "理工学部", examDate: "2027-02-09", tags: ["理工系", "情報系"] },
  ],
  法政大学: [
    { name: "法学部", examDate: "2027-02-16", tags: ["法・政経系"] },
    { name: "文学部", examDate: "2027-02-07", tags: ["文・文化系"] },
    { name: "経済学部", examDate: "2027-02-12", tags: ["法・政経系", "商・経営系"] },
    { name: "経営学部", examDate: "2027-02-07", tags: ["商・経営系"] },
    { name: "社会学部", examDate: "2027-02-12", tags: ["文・文化系"] },
    { name: "国際文化学部", examDate: "2027-02-16", tags: ["文・文化系"] },
    { name: "人間環境学部", examDate: "2027-02-07", tags: ["文・文化系"] },
    { name: "現代福祉学部", examDate: "2027-02-09", tags: ["文・文化系"] },
    { name: "キャリアデザイン学部", examDate: "2027-02-16", tags: ["文・文化系"] },
    { name: "理工学部", examDate: "2027-02-11", tags: ["理工系", "情報系"] },
    { name: "デザイン工学部", examDate: "2027-02-11", tags: ["理工系"] },
    { name: "生命科学部", examDate: "2027-02-14", tags: ["農・生命系"] },
    { name: "情報科学部", examDate: "2027-02-11", tags: ["情報系", "理工系"] },
  ],
};

async function main() {
  // 1. 全国の大学マスターを upsert（name が一意キー）
  for (const u of universities) {
    await prisma.university.upsert({
      where: { name: u.name },
      update: { prefecture: u.prefecture, type: u.type },
      create: { name: u.name, prefecture: u.prefecture, type: u.type },
    });
  }
  console.log(`大学を投入: ${universities.length}校`);

  // 2. 系統タグを upsert（重複作成を避ける）
  const tagNames = [
    "法・政経系",
    "商・経営系",
    "文・文化系",
    "理工系",
    "情報系",
    "医歯薬系",
    "農・生命系",
  ];
  for (const name of tagNames) {
    await prisma.tag.upsert({
      where: { name },
      update: {},
      create: { name },
    });
  }

  // 3. 大学ごとに学部・系統タグを投入（学部データは facultyData を手動拡充）
  for (const [universityName, faculties] of Object.entries(facultyData)) {
    const university = await prisma.university.findUniqueOrThrow({
      where: { name: universityName },
    });

    for (const { name, examDate, tags } of faculties) {
      const existing = await prisma.faculty.findFirst({
        where: { name, universityId: university.id },
      });

      const tagConnect = tags.map((t) => ({ name: t }));

      if (existing) {
        await prisma.faculty.update({
          where: { id: existing.id },
          data: { tags: { set: tagConnect } },
        });
      } else {
        await prisma.faculty.create({
          data: {
            name,
            examDate: new Date(examDate),
            universityId: university.id,
            tags: { connect: tagConnect },
          },
        });
      }
    }
  }

  // 4. デモ用ユーザー（面接官がワンクリックで体験するための共有アカウント）
  const DEMO_EMAIL = "demo@juken-map.com";
  const DEMO_PASSWORD = "demodemo1234";

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

  // Better Auth の認証情報（credential アカウント）を用意
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

  // 5. デモユーザーの志望校（FinalGoal）サンプル
  //    大学名＋学部名から faculty を引いて紐づける
  const demoGoals: {
    university: string;
    faculty: string;
    isFirstChoice: boolean;
    status: "candidate" | "decided";
    note?: string;
  }[] = [
    { university: "早稲田大学", faculty: "政治経済学部", isFirstChoice: true, status: "decided", note: "第一志望。英語と数学を重点的に。" },
    { university: "慶應義塾大学", faculty: "経済学部", isFirstChoice: false, status: "decided", note: "小論文対策が必要。" },
    { university: "明治大学", faculty: "政治経済学部", isFirstChoice: false, status: "decided" },
    { university: "中央大学", faculty: "経済学部", isFirstChoice: false, status: "decided", note: "併願の安全校。" },
    { university: "法政大学", faculty: "経済学部", isFirstChoice: false, status: "candidate", note: "日程が合えば受験候補。" },
    { university: "青山学院大学", faculty: "経済学部", isFirstChoice: false, status: "candidate" },
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
      update: { isFirstChoice: g.isFirstChoice, note: g.note ?? null, status: g.status },
      create: {
        userId: demoUser.id,
        facultyId: faculty.id,
        isFirstChoice: g.isFirstChoice,
        note: g.note ?? null,
        status: g.status,
      },
    });
  }
  console.log(`デモの志望校を投入: ${demoGoals.length}件`);

  // 6. デモユーザーの学習予定（StudyPlan）サンプル
  //    今日を基準にした相対日付。過去は完了済み、今日・未来は未完了にする。
  //    再seedで重複しないよう、デモの既存予定を一度消してから入れ直す（デモのリセット）
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
}

main()
  .then(() => prisma.$disconnect())
  .catch(async (e) => {
    console.error(e);
    await prisma.$disconnect();
    process.exit(1);
  });
