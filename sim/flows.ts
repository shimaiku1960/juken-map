// 画面の操作を、画面が実際に送る順番・中身のまま HTTP にしたもの。
//
// 順番と body は apps/web のコードから確かめてある（詳細はプランの①）。
// 画面を開いたときに自動で走る GET と、保存のあとに追いかける再取得の GET も省かない。
// 省くと、実際より軽いデータと負荷になる。
import { simEmailFor } from "../src/shared/synthetic";
import { Browser, HttpError, type SimApi } from "./client";
import { createRandom, mixSeed, type DayPlan, type Persona } from "./persona";
import type { ResendInbox } from "./resend-inbox";

export const PASSWORD = "synthetic-password";

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

type Textbook = { id: number; name: string; subject: string | null; rangeUnit: string | null };
type StudyPlan = { id: number; date: string; done: boolean; subject: string | null };
type Goal = { id: number };
type University = { id: number; faculties: unknown[] };
type UniversityDetail = { university: { faculties: { id: number }[] } };

/** 画面の「/」を開いたとき（ログイン直後の着地点）。 */
async function openHome(browser: Browser) {
  await browser.request("GET", "/api/auth/get-session");
  await Promise.all([
    browser.request("GET", "/api/goals/first-choice"),
    browser.request("GET", "/api/study-plans"),
    browser.request("GET", "/api/textbooks"),
  ]);
}

/** 画面の「/dashboard」を開いたとき。 */
async function openDashboard(browser: Browser) {
  await browser.request("GET", "/api/auth/get-session");
  const [, , plans] = await Promise.all([
    browser.request("POST", "/api/analytics/registration"),
    browser.request<Goal[]>("GET", "/api/goals"),
    browser.request<StudyPlan[]>("GET", "/api/study-plans"),
    browser.request("GET", "/api/study-logs"),
  ]);
  return plans;
}

/**
 * A. 新規登録。登録→確認メールのリンクを踏む→ログイン→「/」の初期表示まで。
 * 確認メールは実際の利用者と同じく Resend から送られ、その本文を Resend の API で読む。
 */
export async function signUp(browser: Browser, sim: SimApi, inbox: ResendInbox, persona: Persona) {
  const email = simEmailFor(persona.seq);
  const startedAt = new Date();
  await browser.request("POST", "/api/auth/sign-up/email", {
    email,
    password: PASSWORD,
    name: email,
    callbackURL: "/dashboard",
  });
  const verifyPath = await inbox.verificationPath(email, startedAt);
  const verified = await browser.raw("GET", verifyPath);
  await verified.body?.cancel();
  if (verified.status !== 302) {
    throw new HttpError("GET", "/api/auth/verify-email", verified.status, "");
  }
  await sim.markUser(email, { seq: persona.seq, cohort: persona.cohort });
  await signIn(browser, persona.seq);
  await openHome(browser);
}

export async function signIn(browser: Browser, seq: number) {
  browser.cookie = undefined;
  await browser.request("POST", "/api/auth/sign-in/email", {
    email: simEmailFor(seq),
    password: PASSWORD,
  });
}

/**
 * Cookie が切れていたらログインし直す。
 * セッションは7日だが使うたびに延びるので、ほぼ毎日来る人はログインし直さない。
 */
export async function ensureSignedIn(browser: Browser, seq: number) {
  if (browser.cookie) {
    const session = await browser.request<unknown>("GET", "/api/auth/get-session");
    if (session) return false;
  }
  await signIn(browser, seq);
  return true;
}

/** B. 志望校を決める。大学を眺めて学部を1〜3個追加し、1つを第一志望にする。 */
export async function chooseGoals(browser: Browser, random: () => number) {
  // すでに決めてある人は探しに行かない。
  const existing = await browser.request<Goal[]>("GET", "/api/goals");
  if (existing.length > 0) return;

  const universities = await browser.request<University[]>("GET", "/api/universities");
  const withFaculties = universities.filter((u) => u.faculties.length > 0);
  if (withFaculties.length === 0) return;

  const count = 1 + Math.floor(random() * 3);
  for (let i = 0; i < count; i++) {
    const university = withFaculties[Math.floor(random() * withFaculties.length)];
    const [detail] = await Promise.all([
      browser.request<UniversityDetail>("GET", `/api/universities/${university.id}`),
      browser.request("GET", "/api/goals"),
    ]);
    const faculties = detail.university.faculties;
    if (faculties.length === 0) continue;
    const faculty = faculties[Math.floor(random() * faculties.length)];
    const response = await browser.raw("POST", "/api/goals", {
      facultyId: faculty.id,
      status: "candidate",
    });
    await response.body?.cancel();
    // 同じ学部を2回選ぶのは実際にも起きる。409 は正常。
    if (response.status !== 201 && response.status !== 409) {
      throw new HttpError("POST", "/api/goals", response.status, "");
    }
  }

  const goals = await browser.request<Goal[]>("GET", "/api/goals");
  if (goals.length > 0) {
    await browser.request("PATCH", `/api/goals/${goals[0].id}`, { status: "decided" });
    await browser.request("PATCH", `/api/goals/${goals[0].id}`, { isFirstChoice: true });
  }
}

/** C. 毎日の記録。/dashboard を開き、予定を立て、勉強した分を記録する。 */
export async function dailyRecord(
  browser: Browser,
  persona: Persona,
  plan: DayPlan,
  today: string,
  baseSeed: number
) {
  const random = createRandom(mixSeed(baseSeed, persona.seq, Number(today.replaceAll("-", "")), 7));
  const subjectOfDay = () => (random() < 0.6 ? persona.mainSubject : persona.subSubject);

  let plans = await openDashboard(browser);

  if (plan.makesPlan) {
    // 予定の追加ダイアログを開くと参考書一覧を取りに行く。
    let textbooks = await browser.request<Textbook[]>("GET", "/api/textbooks");
    if (textbooks.length === 0 && random() < 0.6) {
      const subject = subjectOfDay();
      const names = TEXTBOOKS[subject];
      const created = await browser.raw("POST", "/api/textbooks", {
        name: names[Math.floor(random() * names.length)],
        subject,
        rangeUnit: "page",
      });
      await created.body?.cancel();
      textbooks = await browser.request<Textbook[]>("GET", "/api/textbooks");
    }
    const textbook = textbooks.length > 0 && random() < 0.6
      ? textbooks[Math.floor(random() * textbooks.length)]
      : undefined;
    const subject = textbook?.subject ?? subjectOfDay();
    await browser.request("POST", "/api/study-plans", {
      date: today,
      items: [
        {
          textbookId: textbook?.id ?? null,
          subject,
          content: textbook ? "" : `${TEXTBOOKS[subject][0]} を進める`,
        },
      ],
    });
    plans = await browser.request<StudyPlan[]>("GET", "/api/study-plans");
  }

  for (let i = 0; i < plan.logCount; i++) {
    const minutes = i === 0 ? plan.minutes : 15 + Math.floor(random() * 60);
    const memo = random() < 0.15 ? MEMOS[Math.floor(random() * MEMOS.length)] : undefined;
    const todayPlan = plans.find((p) => p.date.startsWith(today) && !p.done);
    // 予定があればタイマーから完了する人が多い。完了にし忘れる人もいる。
    if (todayPlan && random() < 0.85) {
      await browser.request("POST", `/api/study-plans/${todayPlan.id}/complete`, {
        minutes,
        ...(memo ? { memo } : {}),
      });
      todayPlan.done = true;
    } else {
      await browser.request("POST", "/api/study-logs", {
        date: today,
        minutes,
        subject: subjectOfDay(),
        ...(memo ? { memo } : {}),
      });
    }
    // 保存のあとは実績と予定の両方が取り直される（useStudyLogs の invalidate）。
    const [, latestPlans] = await Promise.all([
      browser.request("GET", "/api/study-logs"),
      browser.request<StudyPlan[]>("GET", "/api/study-plans"),
    ]);
    plans = latestPlans;
  }

  // D. たまにしか起きないこと
  if (random() < 0.03) {
    await browser.request("PUT", "/api/profile", { nickname: `受験生${persona.seq}` });
  }
}
