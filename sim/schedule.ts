// 「ある日に、誰が何時に来て何をするか」を決める部分。
//
// 毎時の実行（run-hour.ts）と予定の表示（plan.ts）が同じ関数を使う。
// 表示だけ別に計算すると、実際の動きとずれていっても気づけないため。
import {
  createRandom,
  dayPlanFor,
  isPastLife,
  mixSeed,
  personaFor,
  signupHours,
  type DayPlan,
  type Persona,
} from "./persona";

/** /api/sim/state が返す合成ユーザー1人分。 */
export type SimUser = {
  seq: number;
  createdAt: string;
  dormantFrom: string | null;
  lastActedOn: string | null;
};

/** Asia/Tokyo の今日（YYYY-MM-DD）と今の時（0〜23）。 */
export function tokyoNow(now = new Date()) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Tokyo",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    hourCycle: "h23",
  }).formatToParts(now);
  const get = (type: string) => parts.find((p) => p.type === type)!.value;
  return { today: `${get("year")}-${get("month")}-${get("day")}`, hour: Number(get("hour")) };
}

export function tokyoDate(iso: string) {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Tokyo" }).format(new Date(iso));
}

/** 1970-01-01 からの日数。日付の差と、乱数の種に使う。 */
export function dayNumber(ymd: string) {
  const [y, m, d] = ymd.split("-").map(Number);
  return Math.round(Date.UTC(y, m - 1, d) / 86_400_000);
}

export function ymdFromDayNumber(n: number) {
  return new Date(n * 86_400_000).toISOString().slice(0, 10);
}

/** 0=日曜。 */
export function weekdayOf(day: number) {
  return new Date(day * 86_400_000).getUTCDay();
}

/** 志望校を探しに行く確率。登録した日は6割、1週間までは3割、それ以降は行かない。 */
export function goalChance(dayIndex: number) {
  if (dayIndex === 0) return 0.6;
  return dayIndex <= 7 ? 0.3 : 0;
}

export type Visit = { seq: number; persona: Persona; dayIndex: number; plan: DayPlan };

/**
 * その日にアプリを開く既存の利用者（登録した日の人は含めない。登録の流れの中で動くため）。
 * 来なくなった人・寿命を過ぎた人も除く。時間での絞り込みは呼ぶ側でする。
 */
export function visitorsOn(users: SimUser[], day: number, baseSeed: number): Visit[] {
  const weekday = weekdayOf(day);
  const visits: Visit[] = [];
  for (const user of users) {
    if (user.dormantFrom) continue;
    const persona = personaFor(user.seq, baseSeed);
    const dayIndex = day - dayNumber(tokyoDate(user.createdAt));
    if (dayIndex < 1 || isPastLife(persona, dayIndex)) continue;
    const plan = dayPlanFor(persona, dayIndex, day, weekday, baseSeed);
    if (plan.active) visits.push({ seq: user.seq, persona, dayIndex, plan });
  }
  return visits;
}

/**
 * その時間の回で動かす人。予定の時間を過ぎていて、今日まだ動いていない人を予定の早い順に。
 * GitHub Actions の定時実行はよく間引かれるので、予定ちょうどの回だけを見ると来訪ごと消える。
 * 同じ日のうちなら次の回で拾う（日付をまたいだら、その日の来訪は無かったことになる）。
 */
export function dueVisitors(users: SimUser[], day: number, hour: number, baseSeed: number): Visit[] {
  const ymd = ymdFromDayNumber(day);
  const lastActedOn = new Map(users.map((u) => [u.seq, u.lastActedOn]));
  return visitorsOn(users, day, baseSeed)
    .filter(({ seq, plan }) => plan.hour <= hour && lastActedOn.get(seq) !== ymd)
    .sort((a, b) => a.plan.hour - b.plan.hour);
}

/** その日の新規登録の時刻を早い順に。連番はこの順に振られる。 */
export function signupSlots(count: number, day: number, baseSeed: number) {
  return signupHours(count, day, baseSeed).sort((a, b) => a - b);
}

export type PlannedEvent = {
  hour: number;
  seq: number;
  kind: "signup" | "visit";
  persona: Persona;
  plan: DayPlan;
  /** 志望校を探しに行くか（すでに決めてある人は行かない） */
  searchesGoals: boolean;
  /** その日のうちにもう済んだか（登録済み・最終操作日が今日） */
  done: boolean;
};

/**
 * 今日から days 日分の予定。毎時の実行が決めるのと同じ人・同じ時間・同じ中身を返す。
 * まだ登録していない人は、次に振られる連番で仮に数えて、翌日以降の来訪にも含める。
 */
export function planDays(input: {
  users: SimUser[];
  nextSeq: number;
  from: number;
  days: number;
  signupsPerDay: number;
  baseSeed: number;
}) {
  const users = [...input.users];
  let nextSeq = input.nextSeq;
  const result: { day: number; events: PlannedEvent[] }[] = [];

  for (let day = input.from; day < input.from + input.days; day++) {
    const ymd = ymdFromDayNumber(day);
    const searches = (seq: number, dayIndex: number) =>
      createRandom(mixSeed(input.baseSeed, seq, day, 3))() < goalChance(dayIndex);
    const lastActedOn = new Map(users.map((u) => [u.seq, u.lastActedOn]));

    const events: PlannedEvent[] = visitorsOn(users, day, input.baseSeed).map((v) => ({
      hour: v.plan.hour,
      seq: v.seq,
      kind: "visit",
      persona: v.persona,
      plan: v.plan,
      searchesGoals: searches(v.seq, v.dayIndex),
      done: lastActedOn.get(v.seq) === ymd,
    }));

    const joinedToday = users
      .filter((u) => tokyoDate(u.createdAt) === ymd)
      .sort((a, b) => a.seq - b.seq);
    signupSlots(input.signupsPerDay, day, input.baseSeed).forEach((hour, i) => {
      const existing = joinedToday[i];
      const seq = existing?.seq ?? nextSeq++;
      if (!existing) {
        // 翌日以降の来訪を数えるため、登録したことにして足す。
        users.push({ seq, createdAt: `${ymd}T12:00:00+09:00`, dormantFrom: null, lastActedOn: null });
      }
      const persona = personaFor(seq, input.baseSeed);
      events.push({
        hour,
        seq,
        kind: "signup",
        persona,
        plan: dayPlanFor(persona, 0, day, weekdayOf(day), input.baseSeed),
        searchesGoals: searches(seq, 0),
        done: existing !== undefined,
      });
    });

    events.sort((a, b) => a.hour - b.hour || a.seq - b.seq);
    result.push({ day, events });
  }
  return result;
}
