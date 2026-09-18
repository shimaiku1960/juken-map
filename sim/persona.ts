// 合成ユーザー1人1人の「性格」と、ある日にどう動くか。
//
// どちらも連番（seq）と日付だけから決まる純粋関数にしてある。状態を持たないので、
// GitHub Actions のランナーが毎回まっさらでも同じ人は同じように振る舞う。
// DB に残すのは、ここから導けない「実際に起きたこと」（最後に来た日・来なくなった日）だけ。
//
// 続き方の4つの型と割合は db/seed-synthetic.ts の default シナリオと同じ前提に置いた。
// seed は「過去の期間に対する割合」で寿命を決めるが、こちらは毎日積み上げるので
// 寿命を日数で持つ。

export const COHORT_NAMES = ["steady", "fading", "sporadic", "dropped"] as const;
export type CohortName = (typeof COHORT_NAMES)[number];

export const SUBJECTS = ["english", "math", "japanese", "science", "social", "other"] as const;
export type Subject = (typeof SUBJECTS)[number];

/** 種を決めれば同じ並びを返す乱数（mulberry32）。seed-synthetic.ts と同じもの。 */
export function createRandom(seed: number) {
  let state = seed >>> 0;
  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** 複数の整数を1つの種にまとめる。同じ組なら同じ値、少しでも違えば大きく違う値になる。 */
export function mixSeed(...parts: number[]) {
  let h = 0x811c9dc5;
  for (const part of parts) {
    h = Math.imul(h ^ (part >>> 0), 0x01000193);
    h ^= h >>> 13;
  }
  return h >>> 0;
}

const COHORTS: Record<
  CohortName,
  { weight: number; baseRate: number; decay: number; lifeDays: [number, number] }
> = {
  // 続ける人。寿命は実質無限で、1年かけて少しだけ減る。
  steady: { weight: 0.2, baseRate: 0.85, decay: 0.15, lifeDays: [100_000, 100_000] },
  // 最初は熱心だが、だんだん減って途中で止まる。
  fading: { weight: 0.35, baseRate: 0.8, decay: 0.85, lifeDays: [20, 90] },
  // たまに思い出したように使い、いつの間にか来なくなる。
  sporadic: { weight: 0.3, baseRate: 0.25, decay: 0.3, lifeDays: [30, 150] },
  // 登録しただけ。数日で消える。
  dropped: { weight: 0.15, baseRate: 0.6, decay: 0.5, lifeDays: [1, 10] },
};

// 1日のうち何時に使うか（Asia/Tokyo）。受験生は夕方から夜に偏る。
// 数字は相対的な重みで、合計が1である必要は無い。
export const HOUR_WEIGHTS: readonly number[] = [
  2, 1, 0, 0, 0, 0, 1, 2, 1, 1, 1, 1, 2, 1, 1, 1, 2, 3, 3, 4, 6, 7, 6, 4,
];

export type Persona = {
  seq: number;
  cohort: CohortName;
  mainSubject: Subject;
  subSubject: Subject;
  /** 登録から何日で来なくなるか。steady は実質無限。 */
  lifeDays: number;
};

export function sampleHour(random: () => number) {
  const total = HOUR_WEIGHTS.reduce((a, b) => a + b, 0);
  let r = random() * total;
  for (let hour = 0; hour < 24; hour++) {
    r -= HOUR_WEIGHTS[hour];
    if (r < 0) return hour;
  }
  return 21;
}

export function personaFor(seq: number, baseSeed: number): Persona {
  const random = createRandom(mixSeed(baseSeed, seq));
  let r = random();
  let cohort: CohortName = "dropped";
  for (const name of COHORT_NAMES) {
    if (r < COHORTS[name].weight) {
      cohort = name;
      break;
    }
    r -= COHORTS[name].weight;
  }
  const [minLife, maxLife] = COHORTS[cohort].lifeDays;
  return {
    seq,
    cohort,
    mainSubject: SUBJECTS[Math.floor(random() * SUBJECTS.length)],
    subSubject: SUBJECTS[Math.floor(random() * SUBJECTS.length)],
    lifeDays: minLife + Math.floor(random() * (maxLife - minLife + 1)),
  };
}

export type DayPlan = {
  /** 今日アプリを開くか */
  active: boolean;
  /** 開くなら何時か（Asia/Tokyo） */
  hour: number;
  /** 今日の予定を立てるか */
  makesPlan: boolean;
  /** 実績を何件付けるか（0 なら開いて見るだけ） */
  logCount: number;
  /** 学習時間（分）。1件目の実績に使う */
  minutes: number;
};

/**
 * この人がこの日にどう動くか。
 * dayIndex は登録日を0とした経過日数、weekday は 0=日曜。
 */
export function dayPlanFor(
  persona: Persona,
  dayIndex: number,
  dayNumber: number,
  weekday: number,
  baseSeed: number
): DayPlan {
  const random = createRandom(mixSeed(baseSeed, persona.seq, dayNumber));
  const cohort = COHORTS[persona.cohort];
  // 0=登録直後, 1=寿命の終わり。steady は1年で1に届く。
  const span = persona.cohort === "steady" ? 365 : persona.lifeDays;
  const elapsed = Math.min(dayIndex / Math.max(span, 1), 1);
  const weekend = weekday === 0 || weekday === 6;
  const rate = cohort.baseRate * (1 - cohort.decay * elapsed ** 1.5) * (weekend ? 1.15 : 1);

  // 登録した日は必ず開く（登録の流れの続きで画面を見る）。
  const active = dayIndex === 0 || random() < rate;
  const hour = sampleHour(random);
  const makesPlan = random() < 0.45;
  // 開いても記録しない日がある。開いた日の8割は何か記録する。
  const logCount = random() < 0.8 ? (random() < 0.35 ? 2 : 1) : 0;
  const minutes = weekend ? 45 + Math.floor(random() * 136) : 20 + Math.floor(random() * 101);
  return { active, hour, makesPlan, logCount, minutes };
}

/** 寿命を過ぎたか（＝もう来ない人か）。 */
export function isPastLife(persona: Persona, dayIndex: number) {
  return dayIndex > persona.lifeDays;
}

/**
 * その日の新規登録 N 人が、それぞれ何時に登録するか。
 * 時間帯の偏りは利用と同じ曲線を使う。
 */
export function signupHours(count: number, dayNumber: number, baseSeed: number) {
  const random = createRandom(mixSeed(baseSeed, dayNumber, 0x5165));
  return Array.from({ length: count }, () => sampleHour(random));
}
