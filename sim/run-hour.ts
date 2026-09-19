// シミュレーションの1時間分を動かす入口。GitHub Actions から毎時呼ぶ想定。
//
// やること（今の時刻 = Asia/Tokyo の日付と時）:
//   1. /api/sim/state で合成ユーザーの一覧を受け取る
//   2. 寿命を過ぎた人に「来なくなった日」を付ける
//   3. 今日この時間までに使うはずだった人を、ペルソナと日付から決めて、画面と同じ順番で API を叩く
//   4. 今日の新規登録のうち、この時間までに済んでいるべき人数に足りない分を登録する
//
// どれも「今日すでにやったか」を DB の値（最終操作日・登録日時）で判断するので、
// 同じ時間に2回動いても二重にはならない。回が抜けても、同じ日のうちなら次の回で来訪・登録の遅れを取り戻す。
//
// 実行例（手元）:
//   SIM_BASE_URL=http://localhost:4100 SIM_ORIGIN=http://localhost:5173 SIMULATION_SECRET=... \
//     pnpm run sim:run --dry-run --hour 21 --signups 5
// 手順の全体は sim/README.md
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { parseArgs } from "node:util";
import { Browser, HttpError, SharedPacer, SimApi } from "./client";
import { chooseGoals, dailyRecord, ensureSignedIn, signUp } from "./flows";
import { createRandom, dayPlanFor, isPastLife, mixSeed, personaFor, signupHours } from "./persona";
import { ResendInbox } from "./resend-inbox";
import {
  dayNumber,
  dueVisitors,
  goalChance,
  tokyoDate,
  tokyoNow,
  weekdayOf,
  ymdFromDayNumber,
} from "./schedule";

const PRODUCTION_URL = "https://juken-map.com";

const { values: args } = parseArgs({
  options: {
    "dry-run": { type: "boolean", default: false },
    // 既存の利用者のうち、今回動かす人数の上限
    users: { type: "string" },
    // 今日の新規登録の人数（既定は SIGNUPS_PER_DAY）
    signups: { type: "string" },
    // 時刻の上書き。手元でだけ使える（本番では実際の時刻で動く）
    hour: { type: "string" },
  },
});

function env(name: string, fallback?: string) {
  const value = process.env[name] ?? fallback;
  if (value === undefined || value === "") throw new Error(`${name} を設定してください`);
  return value;
}

const baseUrl = env("SIM_BASE_URL").replace(/\/$/, "");
const isLocal = /^http:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/.test(baseUrl);
// 本番以外の外部には向けない。間違った先に合成ユーザーを量産する事故を防ぐ。
if (!isLocal && baseUrl !== PRODUCTION_URL) {
  throw new Error(`SIM_BASE_URL は手元（http://localhost:*）か ${PRODUCTION_URL} だけです（指定: ${baseUrl}）`);
}
if (args.hour !== undefined && !isLocal) {
  throw new Error("--hour は手元でだけ使えます。本番は実際の時刻で動かします");
}

const baseSeed = Number(env("SIM_SEED", "20260918"));
const signupsPerDay = Number(args.signups ?? env("SIGNUPS_PER_DAY", "20"));
const userLimit = args.users === undefined ? Infinity : Number(args.users);
const dryRun = args["dry-run"];
// pnpm sim:run はリポジトリのルートで動くので、そこからの相対で置く。
const stateDir = env("SIM_STATE_DIR", path.join(process.cwd(), "sim", ".state"));
const cookieFile = path.join(stateDir, `cookies-${new URL(baseUrl).host.replace(":", "_")}.json`);

function loadCookies(): Record<string, string> {
  try {
    return JSON.parse(readFileSync(cookieFile, "utf-8"));
  } catch {
    return {};
  }
}

function saveCookies(cookies: Record<string, string>) {
  mkdirSync(stateDir, { recursive: true });
  writeFileSync(cookieFile, JSON.stringify(cookies));
}

async function main() {
  const now = tokyoNow();
  const today = now.today;
  const hour = args.hour === undefined ? now.hour : Number(args.hour);
  const todayNumber = dayNumber(today);
  const weekday = weekdayOf(todayNumber);
  const runId = `${today}T${String(hour).padStart(2, "0")}-${process.pid}`;

  const sim = new SimApi(baseUrl, env("SIMULATION_SECRET"));
  const pacer = new SharedPacer({
    baseUrl,
    // 本番は画面と API が同じ住所。手元で API を別ポートに立てたときは、
    // 画面（Vite, 5173）の住所を SIM_ORIGIN で渡す（Better Auth の trustedOrigins に合わせる）
    origin: env("SIM_ORIGIN", baseUrl).replace(/\/$/, ""),
    runId,
    // Better Auth の組み込み制限は 10 秒 3 回。同じ IP から出るので全員の合計に掛かる。
    authIntervalMs: Number(env("AUTH_INTERVAL_MS", isLocal ? "0" : "4000")),
    requestIntervalMs: Number(env("REQUEST_INTERVAL_MS", isLocal ? "0" : "200")),
  });

  const state = await sim.state();
  const cookies = loadCookies();
  const summary = {
    runId,
    today,
    hour,
    dryRun,
    users: state.users.length,
    becameDormant: 0,
    acted: 0,
    signedIn: 0,
    signedUp: 0,
    errors: [] as string[],
  };

  // 2. 寿命を過ぎた人に「来なくなった日」を付ける
  for (const user of state.users) {
    if (user.dormantFrom) continue;
    const persona = personaFor(user.seq, baseSeed);
    const joinedNumber = dayNumber(tokyoDate(user.createdAt));
    if (!isPastLife(persona, todayNumber - joinedNumber)) continue;
    summary.becameDormant++;
    if (!dryRun) {
      await sim.updateUser(user.seq, {
        dormantFrom: ymdFromDayNumber(joinedNumber + persona.lifeDays + 1),
      });
    }
  }

  // 3. 今日この時間までに使うはずだった人。飛ばされた回の分もここで拾う。
  const due = dueVisitors(state.users, todayNumber, hour, baseSeed).slice(0, userLimit);

  for (const { seq, dayIndex, persona, plan } of due) {
    if (dryRun) {
      console.log(`[dry-run] #${seq} ${persona.cohort} 実績${plan.logCount}件 予定${plan.makesPlan ? "あり" : "なし"}`);
      summary.acted++;
      continue;
    }
    const browser = new Browser(pacer, cookies[seq]);
    try {
      if (await ensureSignedIn(browser, seq)) summary.signedIn++;
      // 登録から1週間は、まだ志望校を決めていなければ探しに行くことがある。
      const random = createRandom(mixSeed(baseSeed, seq, todayNumber, 3));
      if (random() < goalChance(dayIndex)) await chooseGoals(browser, random);
      await dailyRecord(browser, persona, plan, today, baseSeed);
      await sim.updateUser(seq, { lastActedOn: today });
      summary.acted++;
    } catch (error) {
      summary.errors.push(`#${seq}: ${describe(error)}`);
    } finally {
      if (browser.cookie) cookies[seq] = browser.cookie;
    }
  }

  // 4. 新規登録。この時間までに済んでいるべき人数と、今日すでに登録した人数の差だけ登録する。
  const target = signupHours(signupsPerDay, todayNumber, baseSeed).filter((h) => h <= hour).length;
  const signedUpToday = state.users.filter((u) => tokyoDate(u.createdAt) === today).length;
  const toCreate = Math.max(target - signedUpToday, 0);
  const inbox = dryRun ? undefined : new ResendInbox(env("RESEND_READ_API_KEY"));

  let nextSeq = state.nextSeq;
  for (let i = 0; i < toCreate; i++) {
    const persona = personaFor(nextSeq++, baseSeed);
    if (dryRun || !inbox) {
      console.log(`[dry-run] 新規登録 #${persona.seq} ${persona.cohort}`);
      summary.signedUp++;
      continue;
    }
    const browser = new Browser(pacer);
    try {
      await signUp(browser, sim, inbox, persona);
      summary.signedUp++;
      // 登録した日は、そのまま画面を見て少し使う。
      const plan = dayPlanFor(persona, 0, todayNumber, weekday, baseSeed);
      const random = createRandom(mixSeed(baseSeed, persona.seq, todayNumber, 3));
      if (random() < goalChance(0)) await chooseGoals(browser, random);
      await dailyRecord(browser, persona, plan, today, baseSeed);
      await sim.updateUser(persona.seq, { lastActedOn: today });
    } catch (error) {
      summary.errors.push(`新規 #${persona.seq}: ${describe(error)}`);
    } finally {
      if (browser.cookie) cookies[persona.seq] = browser.cookie;
    }
  }

  if (!dryRun) saveCookies(cookies);
  console.log(
    JSON.stringify(
      { ...summary, status: Object.fromEntries(pacer.statusCounts) },
      null,
      2
    )
  );
  // 失敗があれば Actions の実行を赤にして気づけるようにする。
  if (summary.errors.length > 0) process.exitCode = 1;
}

function describe(error: unknown) {
  if (error instanceof HttpError) return error.message;
  return error instanceof Error ? error.message : String(error);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
