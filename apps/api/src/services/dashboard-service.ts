import { shiftYmd, todayYmdTokyo } from "@/shared/date";
import type { Dashboard } from "@/shared/dto/study";
import {
  listDailyStudyMinutes,
  listStudyLogs,
} from "./study-log-service.ts";
import { listStudyPlans } from "./study-plan-service.ts";

/** ダッシュボードが明細を使う直近の日数（今日を含む）。科目別バーと同じ幅。 */
const RECENT_DAYS = 7;
/** 「今週の予定」の幅（今日を含む）。予定は未来にも伸びるので、ここだけ先へ広がる。 */
const UPCOMING_DAYS = 7;
/** 連続記録日数をさかのぼる日数。ここを超える連続は数え切れない。 */
const STREAK_DAYS = 365;

const earlier = (a: string, b: string) => (a < b ? a : b);
const later = (a: string, b: string) => (a > b ? a : b);

/**
 * ダッシュボードの初回表示に要るものを1回でまとめて返す。
 *
 * 別々のAPIにすると1画面で何リクエストにもなり、そのたびにセッション照会が走る。
 * 2026-09-23の実測では、リクエストが3倍になったぶんが応答を小さくした効果を
 * かなり食っていた（素のRPSは2倍でも、利用者から見た改善は+25%だった）。
 *
 * 対象は「今日の月」に固定している。ほかの月はカレンダーが月送りのときに
 * `/api/study-logs` と `/api/study-plans` から取るので、ここで月を選べるようにすると
 * 「その月」と「今日の前後」が離れて、期間が1本につながらなくなる。
 */
export async function getDashboard(userId: string): Promise<Dashboard> {
  const today = todayYmdTokyo();
  const month = today.slice(0, 7);
  const monthStart = `${month}-01`;
  const monthEnd = lastDayOfMonth(month);

  // 実績は過去だけ。直近7日はたいてい当月に収まるので、月の頭の数日だけ前月へ伸びる。
  const logRange = {
    from: earlier(monthStart, shiftYmd(today, -(RECENT_DAYS - 1))),
    to: monthEnd,
  };
  // 予定は未来にもある。今週ぶんが月末をまたぐぶんだけ翌月へ伸びる。
  const planRange = {
    from: monthStart,
    to: later(monthEnd, shiftYmd(today, UPCOMING_DAYS - 1)),
  };

  const [logs, plans, dailyMinutes] = await Promise.all([
    listStudyLogs(userId, logRange),
    listStudyPlans(userId, planRange),
    // 連続記録日数は明細を見ないが、長い期間が要る。日ごとの合計だけを取る。
    listDailyStudyMinutes(userId, { from: shiftYmd(today, -(STREAK_DAYS - 1)) }),
  ]);

  return { month, logRange, logs, planRange, plans, dailyMinutes };
}

/** "YYYY-MM" の末日を "YYYY-MM-DD" で返す。 */
function lastDayOfMonth(month: string): string {
  const [year, m] = month.split("-").map(Number);
  return `${month}-${new Date(year, m, 0).getDate()}`;
}
