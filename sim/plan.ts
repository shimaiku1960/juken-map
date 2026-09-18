// シミュレーションの予定を表示する。何も書き換えない（/api/sim/state を読むだけ）。
//
// 今日と明日、何時に誰が来て何をするか（登録・志望校・予定・記録）を、毎時の実行と
// 同じ計算で出す。今日の分は「済」「飛ばされた」も付くので、実際に動いたかの確認にも使える。
//
// 実行例:
//   SIM_BASE_URL=https://juken-map.com SIMULATION_SECRET=... pnpm run sim:plan
//   pnpm run sim:plan --days 3
import { parseArgs } from "node:util";
import { SimApi } from "./client";
import type { CohortName } from "./persona";
import { dayNumber, planDays, tokyoNow, weekdayOf, ymdFromDayNumber, type PlannedEvent } from "./schedule";

const { values: args } = parseArgs({
  options: {
    days: { type: "string", default: "2" },
    // 1日の新規登録の人数（既定は SIGNUPS_PER_DAY）
    signups: { type: "string" },
  },
});

function env(name: string, fallback?: string) {
  const value = process.env[name] ?? fallback;
  if (value === undefined || value === "") throw new Error(`${name} を設定してください`);
  return value;
}

const COHORT_LABELS: Record<CohortName, string> = {
  steady: "続ける",
  fading: "だんだん減る",
  sporadic: "たまに",
  dropped: "すぐやめる",
};
const WEEKDAYS = ["日", "月", "火", "水", "木", "金", "土"];

function describeActions(event: PlannedEvent) {
  const actions: string[] = [];
  if (event.kind === "signup") actions.push("登録");
  if (event.searchesGoals) actions.push(event.kind === "signup" ? "志望校を選ぶ" : "志望校を選ぶ（未設定なら）");
  if (event.plan.makesPlan) actions.push("予定を立てる");
  if (event.plan.logCount === 1) actions.push(`記録1件（${event.plan.minutes}分）`);
  if (event.plan.logCount > 1) actions.push(`記録${event.plan.logCount}件（1件目${event.plan.minutes}分）`);
  return actions.length > 0 ? actions.join("・") : "開いて見るだけ";
}

function describeStatus(event: PlannedEvent, isToday: boolean, nowHour: number) {
  if (!isToday) return "";
  if (event.done) return "済";
  if (event.hour > nowHour) return "";
  if (event.hour === nowHour) return "この時間";
  // 登録は次の回で取り戻すが、来訪はその時間を逃すとその日は来ない（run-hour.ts）。
  return event.kind === "signup" ? "遅れ（次の回で登録）" : "飛ばされた";
}

async function main() {
  const baseUrl = env("SIM_BASE_URL").replace(/\/$/, "");
  const baseSeed = Number(env("SIM_SEED", "20260918"));
  const signupsPerDay = Number(args.signups ?? env("SIGNUPS_PER_DAY", "20"));
  const days = Number(args.days);

  const state = await new SimApi(baseUrl, env("SIMULATION_SECRET")).state();
  const now = tokyoNow();
  const today = dayNumber(now.today);
  const dormant = state.users.filter((u) => u.dormantFrom).length;

  console.log(
    `${baseUrl}  合成ユーザー ${state.users.length}人（来なくなった人 ${dormant}人）` +
      `  1日の新規登録 ${signupsPerDay}人  今 ${now.today} ${now.hour}時`
  );

  const plans = planDays({ users: state.users, nextSeq: state.nextSeq, from: today, days, signupsPerDay, baseSeed });
  for (const { day, events } of plans) {
    const signups = events.filter((e) => e.kind === "signup").length;
    console.log(
      `\n■ ${ymdFromDayNumber(day)}（${WEEKDAYS[weekdayOf(day)]}）` +
        `  来る ${events.length - signups}人・新規登録 ${signups}人`
    );
    for (const event of events) {
      const status = describeStatus(event, day === today, now.hour);
      console.log(
        [
          `${String(event.hour).padStart(2)}時`,
          `#${event.seq}`.padEnd(6),
          COHORT_LABELS[event.persona.cohort].padEnd(6, "　"),
          describeActions(event),
          status && `［${status}］`,
        ]
          .filter(Boolean)
          .join("  ")
      );
    }
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
