// 「実際に多くの人が使っている状態」を手元で再現する seed。
//
// 目的は2つあって、どちらも同じデータで足りる。
//   - 可観測性：Grafana のグラフや Tempo のトレースが、1人分の練習データではなく
//     実運用に近い量と偏りの上で読めるようにする。
//   - 負荷試験：k6 から実際にログインできる利用者が大量に要る。
//
// 大事なのは行数ではなく偏り。全員が毎日きっちり記録する前提で作ると、
// 索引もキャッシュも実際より効いてしまい、測っても意味のある数字にならない。
// ここでは続く人・だんだん減る人・たまに使う人・登録しただけの人を混ぜる。
//
// 実行例:
//   pnpm db:seed:synthetic                    # 300人 × 6か月
//   USERS=1000 MONTHS=12 pnpm db:seed:synthetic
//
// 同じ SEED なら何度流しても同じデータになる（毎回入れ直す）。
import { generateId } from "better-auth";
import { hashPassword } from "better-auth/crypto";
import { execute, runSeed, select } from "./seed-helpers";

const USERS = Number(process.env.USERS ?? 300);
const MONTHS = Number(process.env.MONTHS ?? 6);
const SEED = Number(process.env.SEED ?? 20260917);
const PASSWORD = process.env.SYNTHETIC_PASSWORD ?? "synthetic-password";

// 合成データだけを狙って消せるように、メールアドレスに印を付ける。
// .invalid は「実在しないことが保証されたTLD」（RFC 2606）で、誤って実メールを送らない。
const EMAIL_DOMAIN = "@synthetic.juken-map.invalid";
const DAYS = Math.round(MONTHS * 30.4);

/**
 * 種を決めれば同じ並びを返す乱数（mulberry32）。
 * Math.random だと実行のたびに中身が変わり、「昨日より遅い」の比較ができない。
 */
function createRandom(seed: number) {
  let state = seed >>> 0;
  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const random = createRandom(SEED);
const pick = <T>(items: readonly T[]) => items[Math.floor(random() * items.length)];
const randInt = (min: number, max: number) => min + Math.floor(random() * (max - min + 1));

/** 今日から n 日前（未来は負でない値）の UTC 0 時。helpers と同じ保存の仕方に揃える。 */
function dayAt(offset: number) {
  const base = new Date();
  const utc = Date.UTC(base.getFullYear(), base.getMonth(), base.getDate());
  return new Date(utc + offset * 86400000);
}

// 続き方の型。割合は「受験生向けサービスなら大半は続かない」という前提で置いた。
// 実際の数字が分かったら、ここを現実に寄せていく。
const COHORTS = [
  // 続ける人。今日まで記録が続く。
  { name: "steady", weight: 0.2, baseRate: 0.85, decay: 0.15, life: [1, 1] },
  // 最初は熱心だが、だんだん減って途中で止まる。一番多い。
  { name: "fading", weight: 0.35, baseRate: 0.8, decay: 0.85, life: [0.2, 0.6] },
  // たまに思い出したように使い、いつの間にか来なくなる。
  { name: "sporadic", weight: 0.3, baseRate: 0.25, decay: 0.3, life: [0.3, 1] },
  // 登録しただけ。数日で消える。
  { name: "dropped", weight: 0.15, baseRate: 0.6, decay: 0.5, life: [0.02, 0.1] },
] as const;

function pickCohort() {
  let r = random();
  for (const cohort of COHORTS) {
    if (r < cohort.weight) return cohort;
    r -= cohort.weight;
  }
  return COHORTS[COHORTS.length - 1];
}

const SUBJECTS = ["english", "math", "japanese", "science", "social", "other"] as const;

// 参考書は科目に紐づけて選ぶ。名前だけの一覧で、マスター（TextbookMaster）には触らない。
const TEXTBOOKS: Record<string, string[]> = {
  english: ["ターゲット1900", "Vintage", "ポレポレ英文読解", "やっておきたい英語長文300"],
  math: ["青チャート数学ⅠA", "基礎問題精講ⅡB", "1対1対応の演習", "文系数学の良問プラチカ"],
  japanese: ["現代文読解力の開発講座", "古文単語ゴロゴ", "漢文早覚え速答法"],
  science: ["リードα物理", "化学重要問題集", "セミナー生物"],
  social: ["日本史B一問一答", "世界史用語集", "蔭山の共通テスト政治経済"],
  other: ["共通テスト過去問", "小論文の書き方"],
};

const MEMOS = [
  "集中できた",
  "眠くて進まなかった",
  "間違えたところを明日やり直す",
  "思ったより時間がかかった",
  "苦手なところが分かった",
];

type Row = unknown[];

/** 行をまとめて INSERT する。1本のSQLに詰め込みすぎると max_allowed_packet に当たるので区切る。 */
async function insertMany(table: string, columns: string[], rows: Row[], chunkSize = 400) {
  if (rows.length === 0) return;
  const placeholders = `(${columns.map(() => "?").join(", ")})`;
  for (let i = 0; i < rows.length; i += chunkSize) {
    const chunk = rows.slice(i, i + chunkSize);
    await execute(
      `INSERT INTO \`${table}\` (${columns.map((c) => `\`${c}\``).join(", ")})
       VALUES ${chunk.map(() => placeholders).join(", ")}`,
      chunk.flat()
    );
  }
}

runSeed(async () => {
  // 本番のRDSに合成データを流し込む事故を防ぐ。意図的にやるときだけ ALLOW_REMOTE=on。
  const host = new URL(process.env.DATABASE_URL ?? "mysql://localhost").hostname;
  const isLocal = ["localhost", "127.0.0.1", "::1", "db"].includes(host);
  if (!isLocal && process.env.ALLOW_REMOTE !== "on") {
    throw new Error(`手元のDB以外（${host}）には流しません。意図的に流すなら ALLOW_REMOTE=on を付けてください`);
  }

  const faculties = await select<{ id: number }>("SELECT id FROM Faculty ORDER BY id ASC");
  if (faculties.length === 0) {
    throw new Error("学部マスターが空です。先に pnpm db:seed を実行してください");
  }

  // 入れ直し。user を消せば実績・予定・参考書・志望校も FK の CASCADE で消える。
  const [{ removed }] = await select<{ removed: number }>(
    "SELECT COUNT(*) AS removed FROM `user` WHERE email LIKE ?",
    [`%${EMAIL_DOMAIN}`]
  );
  await execute("DELETE FROM `user` WHERE email LIKE ?", [`%${EMAIL_DOMAIN}`]);
  if (removed > 0) console.log(`前回の合成ユーザーを削除: ${removed}人`);

  // パスワードのハッシュは意図的に重い（scrypt）。全員同じパスワードなので1回だけ計算して使い回す。
  // 1人ずつ計算すると、ここだけで数分かかる。
  const passwordHash = await hashPassword(PASSWORD);

  const users: Row[] = [];
  const accounts: Row[] = [];
  const goals: Row[] = [];
  const textbooks: { userId: string; name: string; subject: string; totalAmount: number }[] = [];
  const preferences: Row[] = [];
  // 予定と実績は id を採番してから紐づけたいので、いったん貯めて後でまとめて入れる
  const plans: { userId: string; offset: number; subject: string; content: string; done: boolean }[] = [];
  const logs: {
    userId: string;
    offset: number;
    subject: string;
    minutes: number;
    memo: string | null;
    planIndex: number | null;
    textbookIndex: number | null;
    rangeStart: number | null;
    rangeEnd: number | null;
  }[] = [];

  for (let i = 0; i < USERS; i++) {
    const userId = generateId();
    const cohort = pickCohort();
    // 登録日。全員が同時に始めるわけではないので散らす。
    const joinedOffset = -randInt(7, DAYS);
    // 主に使う科目。人によって偏りがある方が、科目別の集計が現実に近くなる。
    const mainSubject = pick(SUBJECTS);
    const subSubject = pick(SUBJECTS);
    const now = new Date();
    const joinedAt = dayAt(joinedOffset);

    users.push([
      userId,
      `synthetic${String(i + 1).padStart(5, "0")}${EMAIL_DOMAIN}`,
      `合成ユーザー${i + 1}`,
      `ユーザー${i + 1}`,
      true,
      joinedAt,
      now,
    ]);
    accounts.push([generateId(), userId, userId, "credential", passwordHash, joinedAt, now]);

    // 参考書（この人の主科目に寄せる）
    const bookCount = randInt(1, 4);
    const myBooks: number[] = [];
    // 参考書は前回の続きから進む。毎回1ページ目に戻るデータでは「どこまで終わったか」が出ない。
    const bookProgress = new Map<number, number>();
    const usedNames = new Set<string>();
    for (let b = 0; b < bookCount; b++) {
      const subject = random() < 0.6 ? mainSubject : pick(SUBJECTS);
      const name = pick(TEXTBOOKS[subject]);
      if (usedNames.has(name)) continue; // Textbook は (userId, name) が UNIQUE
      usedNames.add(name);
      myBooks.push(textbooks.length);
      textbooks.push({ userId, name, subject, totalAmount: randInt(100, 600) });
    }

    // 志望校。第一志望を1つ決め、残りは併願（一部は検討中のまま）。
    const goalCount = randInt(1, 6);
    const chosen = new Set<number>();
    for (let g = 0; g < goalCount; g++) {
      const faculty = pick(faculties);
      if (chosen.has(faculty.id)) continue; // (userId, facultyId) が UNIQUE
      chosen.add(faculty.id);
      goals.push([
        userId,
        faculty.id,
        g === 0,
        g === 0 ? "第一志望" : null,
        random() < 0.25 ? "candidate" : "decided",
        joinedAt,
      ]);
    }

    // 通知の設定。全員がONにするわけではない。
    if (random() < 0.4) {
      preferences.push([userId, random() < 0.7, random() < 0.5, joinedAt, now]);
    }

    // 日ごとの活動。
    // 「だんだん減る」（decay）と「ある日から完全に来なくなる」（離脱）は別物で、
    // 両方いる。減衰だけで作ると全員がうっすら生きているデータになり、
    // 継続率が実際よりずっと良く見えてしまう。
    const activeDays = -joinedOffset;
    const life = cohort.life[0] + random() * (cohort.life[1] - cohort.life[0]);
    const lastActiveOffset = Math.min(0, joinedOffset + Math.ceil(activeDays * life));
    for (let offset = joinedOffset; offset <= lastActiveOffset; offset++) {
      // 0=登録直後, 1=離脱の直前
      const elapsed = (offset - joinedOffset) / Math.max(lastActiveOffset - joinedOffset, 1);
      // 減衰。dropped は最初の1〜2週間で 0 に近づく。
      const decayed = cohort.baseRate * (1 - cohort.decay * elapsed ** 1.5);
      // 曜日の偏り。土日は勉強時間が増える。
      const weekday = dayAt(offset).getUTCDay();
      const weekendBoost = weekday === 0 || weekday === 6 ? 1.15 : 1;
      // 受験が近づくほど全体的に増える（今日に近いほど少し上がる）
      const seasonBoost = 1 + 0.15 * elapsed;
      const rate = Math.max(0, decayed * weekendBoost * seasonBoost);
      if (random() >= rate) {
        // 勉強しなかった日。予定だけ立てて手を付けなかった、は実際によく起きる。
        // ここが無いと「予定は必ず実行される」データになり、未完了の予定を探す
        // 通知バッチのクエリが現実より軽くなってしまう。
        if (cohort.name !== "dropped" && random() < 0.12) {
          const subject = random() < 0.6 ? mainSubject : subSubject;
          plans.push({
            userId,
            offset,
            subject,
            content: `${pick(TEXTBOOKS[subject])} を進める`,
            done: false,
          });
        }
        continue;
      }

      // 予定を立ててから勉強する人ばかりではない
      let planIndex: number | null = null;
      if (random() < 0.45) {
        const subject = random() < 0.6 ? mainSubject : subSubject;
        planIndex = plans.length;
        plans.push({
          userId,
          offset,
          subject,
          content: `${pick(TEXTBOOKS[subject])} を進める`,
          // 勉強はしたのに完了にし忘れる人もいる
          done: random() < 0.85,
        });
      }

      const logCount = random() < 0.35 ? 2 : random() < 0.1 ? 3 : 1;
      for (let l = 0; l < logCount; l++) {
        const subject = l === 0 ? (random() < 0.6 ? mainSubject : subSubject) : pick(SUBJECTS);
        // 平日は短め、休日は長め。たまに長時間。
        const base = weekendBoost > 1 ? randInt(45, 180) : randInt(20, 120);
        const textbookIndex = myBooks.length > 0 && random() < 0.6 ? pick(myBooks) : null;

        // 参考書を使ったときだけ、前回の続きから何ページか進める
        let rangeStart: number | null = null;
        let rangeEnd: number | null = null;
        if (textbookIndex !== null) {
          const total = textbooks[textbookIndex].totalAmount;
          const done = bookProgress.get(textbookIndex) ?? 0;
          if (done < total) {
            rangeStart = done + 1;
            rangeEnd = Math.min(total, rangeStart + randInt(2, 20));
            bookProgress.set(textbookIndex, rangeEnd);
          }
        }

        logs.push({
          userId,
          offset,
          subject,
          minutes: random() < 0.05 ? base + randInt(60, 180) : base,
          memo: random() < 0.15 ? pick(MEMOS) : null,
          planIndex: l === 0 ? planIndex : null,
          textbookIndex,
          rangeStart,
          rangeEnd,
        });
      }
    }

    // これからの予定（未来日）。使っている人ほど先の予定を入れている。
    if (lastActiveOffset >= -3) {
      const futureCount = randInt(0, cohort.name === "steady" ? 8 : 3);
      for (let f = 0; f < futureCount; f++) {
        const subject = random() < 0.6 ? mainSubject : pick(SUBJECTS);
        plans.push({
          userId,
          offset: randInt(0, 14),
          subject,
          content: `${pick(TEXTBOOKS[subject])} を進める`,
          done: false,
        });
      }
    }
  }

  const now = new Date();
  await insertMany(
    "user",
    ["id", "email", "name", "nickname", "emailVerified", "createdAt", "updatedAt"],
    users
  );
  console.log(`ユーザーを投入: ${users.length}人`);

  await insertMany(
    "account",
    ["id", "userId", "accountId", "providerId", "password", "createdAt", "updatedAt"],
    accounts
  );

  await insertMany(
    "FinalGoal",
    ["userId", "facultyId", "isFirstChoice", "note", "status", "createdAt"],
    goals
  );
  console.log(`志望校を投入: ${goals.length}件`);

  await insertMany(
    "Textbook",
    ["userId", "name", "subject", "totalAmount", "rangeUnit", "createdAt", "updatedAt"],
    textbooks.map((book) => [book.userId, book.name, book.subject, book.totalAmount, "ページ", now, now])
  );
  console.log(`参考書を投入: ${textbooks.length}件`);

  if (preferences.length > 0) {
    await insertMany(
      "NotificationPreference",
      ["userId", "morningEnabled", "eveningEnabled", "createdAt", "updatedAt"],
      preferences
    );
    console.log(`通知の設定を投入: ${preferences.length}件`);
  }

  await insertMany(
    "StudyPlan",
    ["userId", "date", "content", "subject", "done", "createdAt", "updatedAt"],
    plans.map((plan) => [plan.userId, dayAt(plan.offset), plan.content, plan.subject, plan.done, now, now])
  );
  console.log(`学習予定を投入: ${plans.length}件`);

  // 実績から予定・参考書へ繋ぐために、採番された id を取り直す。
  // 入れた順に id が増えるので、投入順の配列とそのまま対応させられる。
  const planIds = await select<{ id: number }>(
    `SELECT p.id FROM StudyPlan AS p
     JOIN \`user\` AS u ON u.id = p.userId
     WHERE u.email LIKE ? ORDER BY p.id ASC`,
    [`%${EMAIL_DOMAIN}`]
  );
  const bookIds = await select<{ id: number }>(
    `SELECT t.id FROM Textbook AS t
     JOIN \`user\` AS u ON u.id = t.userId
     WHERE u.email LIKE ? ORDER BY t.id ASC`,
    [`%${EMAIL_DOMAIN}`]
  );

  await insertMany(
    "StudyLog",
    [
      "userId", "date", "subject", "minutes", "memo",
      "studyPlanId", "textbookId", "rangeStart", "rangeEnd", "rangeUnit",
      "createdAt", "updatedAt",
    ],
    logs.map((log) => [
      log.userId,
      dayAt(log.offset),
      log.subject,
      log.minutes,
      log.memo,
      log.planIndex === null ? null : (planIds[log.planIndex]?.id ?? null),
      log.textbookIndex === null ? null : (bookIds[log.textbookIndex]?.id ?? null),
      log.rangeStart,
      log.rangeEnd,
      log.rangeStart === null ? null : "ページ",
      now,
      now,
    ])
  );
  console.log(`学習実績を投入: ${logs.length}件`);

  // 「初回の記録」の印を、実際の最初の実績に合わせる（通知やお祝いの分岐が現実的になる）
  await execute(
    `UPDATE \`user\` AS u
     JOIN (SELECT userId, MIN(date) AS firstDate FROM StudyLog GROUP BY userId) AS l ON l.userId = u.id
     SET u.firstStudyLogAt = l.firstDate
     WHERE u.email LIKE ?`,
    [`%${EMAIL_DOMAIN}`]
  );

  console.log("");
  console.log(`k6 などからログインする場合:`);
  console.log(`  メール: synthetic00001${EMAIL_DOMAIN} 〜 synthetic${String(USERS).padStart(5, "0")}${EMAIL_DOMAIN}`);
  console.log(`  パスワード: ${PASSWORD}（全員共通）`);
});
