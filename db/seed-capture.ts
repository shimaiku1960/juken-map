// LPのスクリーンショット・動画撮影専用ユーザーを、ローカルDBへ再現可能な状態で投入する。
// 共有デモユーザーとは分離し、タイマー開始→実績保存まで実際に操作できるようにする。
// 実行: pnpm run capture:seed
import {
  execute,
  findFacultyId,
  insertStudyLogs,
  insertStudyPlans,
  runSeed,
  setCredentialPassword,
  upsertVerifiedUser,
} from "./seed-helpers";

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

// 受験日程は第一志望だけだと1行で寂しいため、LPの撮影用に併願校まで揃える。
// 受験日はマスター（Faculty.examDate）が持つので、ここでは大学・学部名だけ指定する。
const goalSpecs = [
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

// 動画開始時点は今日1時間45分。英語30分を保存すると、LPの訴求値と同じ
// 「今日2時間15分・英語5時間10分・8日連続」になる。
const studyLogs = [
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

const studyPlans = [
  { offset: 0, content: "英語長文 1題（早稲田過去問）", subject: "english", done: false },
  { offset: 0, content: "数学ⅠA 二次関数 演習", subject: "math", done: true },
  { offset: 0, content: "現代文 評論 読解1題", subject: "japanese", done: true },
  { offset: 1, content: "日本史 近現代 通史", subject: "social", done: false },
  { offset: 3, content: "英文法 Vintage 仮定法", subject: "english", done: false },
];

runSeed(async () => {
  const userId = await upsertVerifiedUser(
    { email: CAPTURE_EMAIL, name: "撮影用ユーザー", nickname: "受験マップ太郎" },
    { resetProfile: true }
  );
  await setCredentialPassword(userId, CAPTURE_PASSWORD);

  // 撮影を毎回同じ初期状態から始めるため、撮影ユーザーの可変データだけをリセットする。
  await execute("DELETE FROM session WHERE userId = ?", [userId]);
  await execute("DELETE FROM StudyLog WHERE userId = ?", [userId]);
  await execute("DELETE FROM StudyPlan WHERE userId = ?", [userId]);
  await execute("DELETE FROM FinalGoal WHERE userId = ?", [userId]);

  const now = new Date();
  for (const spec of goalSpecs) {
    const facultyId = await findFacultyId(spec.university, spec.faculty);
    // decided かつ isFirstChoice=false が「併願校」として表示される。
    await execute(
      `INSERT INTO FinalGoal (userId, facultyId, isFirstChoice, note, status, createdAt)
       VALUES (?, ?, ?, ?, 'decided', ?)`,
      [userId, facultyId, spec.isFirstChoice, spec.note, now]
    );
  }

  await insertStudyLogs(userId, studyLogs);
  await insertStudyPlans(userId, studyPlans);

  console.log("LP撮影用データを初期状態へ戻しました");
  console.log(`メールアドレス: ${CAPTURE_EMAIL}`);
  console.log(`パスワード: ${CAPTURE_PASSWORD}`);
  console.log("動画では英語長文を30分として保存してください");
});
