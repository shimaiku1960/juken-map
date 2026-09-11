// ローカル・CI の DB を初期状態にする seed。大学マスター（全国の大学・一部の学部・系統タグ）と
// デモユーザーを入れる。何度流しても同じ状態になる（upsert とデモのリセット）。
// 実行: pnpm run db:seed
import { readFileSync } from "fs";
import { seedDemoUser } from "./demo-user";
import { execute, runSeed, select } from "./seed-helpers";

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

// 多くの行を1本の INSERT で入れるときの1回あたりの行数（プレースホルダが増えすぎないように）
const CHUNK_SIZE = 200;

const TAG_NAMES = [
  "法・政経系",
  "商・経営系",
  "文・文化系",
  "理工系",
  "情報系",
  "医歯薬系",
  "農・生命系",
];

async function main() {
  const now = new Date();

  // 1. 全国の大学マスターを upsert（name が UNIQUE）。
  //    Prisma は1校ずつ「探す → 作成か更新」をしていた（823校で約2,500往復）。
  //    ここでは複数行の INSERT ... ON DUPLICATE KEY UPDATE を数本流すだけで済ませる。
  //    University の UNIQUE は name だけ（id は自動採番で指定しない）なので、ON DUPLICATE KEY で安全。
  for (let i = 0; i < universities.length; i += CHUNK_SIZE) {
    const chunk = universities.slice(i, i + CHUNK_SIZE);
    await execute(
      `INSERT INTO University (name, prefecture, type, createdAt)
       VALUES ${chunk.map(() => "(?, ?, ?, ?)").join(", ")} AS new
       ON DUPLICATE KEY UPDATE prefecture = new.prefecture, type = new.type`,
      chunk.flatMap((u) => [u.name, u.prefecture, u.type, now])
    );
  }
  console.log(`大学を投入: ${universities.length}校`);

  // 2. 系統タグ。既にあれば何もしない（name = name は「更新なし」の書き方）
  await execute(
    `INSERT INTO Tag (name, createdAt) VALUES ${TAG_NAMES.map(() => "(?, ?)").join(", ")} AS new
     ON DUPLICATE KEY UPDATE name = Tag.name`,
    TAG_NAMES.flatMap((name) => [name, now])
  );

  // 3. 大学ごとに学部と系統タグを投入（学部データは facultyData を手動で拡充する）。
  //    既存の学部はタグの付け替えだけ行い、受験日は変えない（Prisma 版と同じ）。
  for (const [universityName, faculties] of Object.entries(facultyData)) {
    const [university] = await select<{ id: number }>(
      "SELECT id FROM University WHERE name = ?",
      [universityName]
    );
    if (!university) throw new Error(`${universityName} が大学マスターにありません`);

    for (const { name, examDate, tags } of faculties) {
      const [existing] = await select<{ id: number }>(
        "SELECT id FROM Faculty WHERE name = ? AND universityId = ? ORDER BY id ASC LIMIT 1",
        [name, university.id]
      );

      let facultyId: number;
      if (existing) {
        facultyId = existing.id;
        // タグを指定どおりに置き換える（Prisma の set）。中間テーブルの A = Faculty.id, B = Tag.id
        await execute("DELETE FROM _FacultyToTag WHERE A = ?", [facultyId]);
      } else {
        const created = await execute(
          "INSERT INTO Faculty (name, examDate, universityId, createdAt) VALUES (?, ?, ?, ?)",
          [name, new Date(examDate), university.id, now]
        );
        facultyId = created.insertId;
      }
      await execute(
        "INSERT INTO _FacultyToTag (A, B) SELECT ?, id FROM Tag WHERE name IN (?)",
        [facultyId, tags]
      );
    }
  }

  // 4〜6. デモユーザーと、その志望校・学習予定
  await seedDemoUser({ withLogs: false });
}

runSeed(main);
