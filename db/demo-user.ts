// デモユーザー（面接官がワンクリックで体験する共有・閲覧専用アカウント）の投入。
// seed.ts（ローカル・CI の初期化）と seed-demo.ts（本番へデモだけ入れ直す）の両方から使う。
// 大学マスターには触れず、既存の学部を名前で引いて紐づける。
import {
  execute,
  findFacultyId,
  insertStudyLogs,
  insertStudyPlans,
  setCredentialPassword,
  upsertVerifiedUser,
} from "./seed-helpers";

const DEMO_EMAIL = "demo@juken-map.com";
const DEMO_PASSWORD = "demodemo1234";

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
  // 比較検討中の候補（受験校としては未確定）
  { university: "法政大学", faculty: "経済学部", isFirstChoice: false, status: "candidate", note: "日程が合えば受験候補。" },
  { university: "青山学院大学", faculty: "経済学部", isFirstChoice: false, status: "candidate" },
];

// 今日を基準にした相対日付。過去は完了済み、今日・未来は未完了にする。
const demoPlans = [
  { offset: -3, content: "英単語 ターゲット1900（前半）", subject: "english", done: true },
  { offset: -3, content: "数学ⅠA 二次関数 演習", subject: "math", done: true },
  { offset: -1, content: "現代文 評論 読解1題", subject: "japanese", done: true },
  { offset: 0, content: "英語長文 1題（早稲田過去問）", subject: "english", done: false },
  { offset: 0, content: "日本史 近現代 通史", subject: "social", done: false },
  { offset: 1, content: "数学ⅡB ベクトル", subject: "math", done: false },
  { offset: 3, content: "英文法 Vintage 仮定法", subject: "english", done: false },
  { offset: 6, content: "古文 助動詞 暗記", subject: "japanese", done: false },
];

// 直近2週間ぶん。ヒートマップ・ストリーク・科目別バーが「動いて見える」よう科目と時間を散らす。
const demoLogs = [
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

/**
 * デモユーザーと、その志望校・学習予定（withLogs なら学習実績も）を入れ直す。
 * 予定と実績は再実行で重複しないよう、デモの分だけ消してから入れる（デモのリセット）。
 */
export async function seedDemoUser(options: { withLogs: boolean }) {
  const userId = await upsertVerifiedUser({
    email: DEMO_EMAIL,
    name: "デモユーザー",
    nickname: "デモ太郎",
  });
  await setCredentialPassword(userId, DEMO_PASSWORD);
  console.log(`デモユーザーを投入: ${DEMO_EMAIL}`);

  const now = new Date();
  for (const goal of demoGoals) {
    const facultyId = await findFacultyId(goal.university, goal.faculty);
    // FinalGoal の UNIQUE は (userId, facultyId) だけなので、ON DUPLICATE KEY で安全に upsert できる
    await execute(
      `INSERT INTO FinalGoal (userId, facultyId, isFirstChoice, note, status, createdAt)
       VALUES (?, ?, ?, ?, ?, ?) AS new
       ON DUPLICATE KEY UPDATE
         isFirstChoice = new.isFirstChoice, note = new.note, status = new.status`,
      [userId, facultyId, goal.isFirstChoice, goal.note ?? null, goal.status, now]
    );
  }
  console.log(`デモの志望校を投入: ${demoGoals.length}件`);

  await execute("DELETE FROM StudyPlan WHERE userId = ?", [userId]);
  await insertStudyPlans(userId, demoPlans);
  console.log(`デモの学習予定を投入: ${demoPlans.length}件`);

  if (options.withLogs) {
    await execute("DELETE FROM StudyLog WHERE userId = ?", [userId]);
    await insertStudyLogs(userId, demoLogs);
    console.log(`デモの学習実績を投入: ${demoLogs.length}件`);
  }
}
