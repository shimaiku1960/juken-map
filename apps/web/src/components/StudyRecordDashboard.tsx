import { useRef } from "react";
import { useDailyStudyMinutes, useStudyLogs } from "@/web/hooks/useStudyLogs";
import { useStudyPlans, type StudyPlan } from "@/web/hooks/useStudyPlans";
import {
  computeStreak,
  computeSubjectMinutes,
} from "@/shared/studyStats";
import { shiftYmd, ymdLocal, todayYmd, ymdAfterDays } from "@/shared/date";
import { formatMinutes } from "@/web/lib/studyLog";
import StreakBadge from "@/web/components/StreakBadge";
import StudyHeatmap, { type StudyHeatmapHandle } from "@/web/components/StudyHeatmap";
import SubjectMinutesBars from "@/web/components/SubjectMinutesBars";
import TodayStudyPlans from "@/web/components/TodayStudyPlans";
import { Card, CardContent } from "@/web/components/ui/card";
import { studyPlanLabel } from "@/web/lib/studyPlan";

// この画面が明細を要るのは「今日」と「直近7日」だけ。
// 連続記録日数はもっと遡る必要があるが、日ごとの合計しか見ないので別の軽い API から取る。
const RECENT_DAYS = 7;
const STREAK_DAYS = 365;

// 実績（StudyLog）まわりのダッシュボード。記録するとキャッシュ更新で
// ストリーク・ヒートマップ・科目別バーが即座に伸びる（クライアントで集計）。
export default function StudyRecordDashboard({
  initialPlans,
  readOnly = false,
}: {
  // SPA では初期データをサーバーから渡せないので任意にする。
  // ここで [] を既定値にすると TanStack Query が initialData ありと判断して
  // staleTime の間フェッチせず、空のまま表示されてしまうので undefined を通す。
  initialPlans?: StudyPlan[];
  readOnly?: boolean;
}) {
  const today = todayYmd();
  // 直近7日間（今日を含む）
  const weekFrom = ymdAfterDays(-(RECENT_DAYS - 1));
  const { data: logs = [], isPending: logsLoading } = useStudyLogs({
    from: weekFrom,
  });
  const { data: dailyMinutes = [] } = useDailyStudyMinutes({
    from: shiftYmd(today, -(STREAK_DAYS - 1)),
  });
  const {
    data: plans = [],
    isPending: plansLoading,
    isError: plansError,
    refetch: refetchPlans,
  } = useStudyPlans(initialPlans);

  const heatmapRef = useRef<StudyHeatmapHandle>(null);
  const streak = computeStreak(dailyMinutes, today);

  const todayMinutes = logs
    .filter((l) => ymdLocal(l.date) === today)
    .reduce((sum, l) => sum + l.minutes, 0);

  const recordedMinutesByPlan = new Map<number, number>();
  for (const log of logs) {
    if (log.studyPlanId == null) continue;
    recordedMinutesByPlan.set(
      log.studyPlanId,
      (recordedMinutesByPlan.get(log.studyPlanId) ?? 0) + log.minutes
    );
  }
  const todayPlans = plans
    .filter((plan) => plan.date.slice(0, 10) === today)
    .map((plan) => {
      const recordedMinutes = recordedMinutesByPlan.get(plan.id) ?? 0;
      return {
        id: plan.id,
        label: studyPlanLabel(plan),
        done: plan.done,
        recordedMinutes: recordedMinutes > 0 ? recordedMinutes : null,
      };
    });
  const weekThrough = ymdAfterDays(6);
  const weekPlanCount = plans.filter((plan) => {
    const date = plan.date.slice(0, 10);
    return date >= today && date <= weekThrough;
  }).length;

  // 直近7日間（今日を含む）の科目別合計
  const subjectMinutes = computeSubjectMinutes(logs, weekFrom, today);

  const showTodayInCalendar = (openCreate: boolean) => {
    heatmapRef.current?.showToday(openCreate);
    requestAnimationFrame(() => {
      document.getElementById("study-calendar")?.scrollIntoView({ behavior: "smooth" });
    });
  };

  return (
    <>
      <section className="mb-8" aria-labelledby="today-study-plans-heading">
        <h2 id="today-study-plans-heading" className="mb-3 text-xl font-bold">
          今日やること
        </h2>
        <TodayStudyPlans
          plans={todayPlans}
          weekCount={weekPlanCount}
          hasError={plansError}
          isLoading={plansLoading}
          readOnly={readOnly}
          onRetry={() => void refetchPlans()}
          onShowToday={showTodayInCalendar}
        />
      </section>

      <section id="study-calendar" className="mb-8 scroll-mt-24">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 className="text-xl font-bold">学習カレンダー</h2>
          <StreakBadge streak={streak} />
        </div>
        <Card>
          <CardContent className="px-3 py-5 sm:px-6">
            {plansError ? (
              <div className="mb-4 flex flex-wrap items-center justify-between gap-2 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
                <span>学習予定を更新できませんでした。表示が古い可能性があります。</span>
                <button
                  type="button"
                  className="font-medium underline underline-offset-2"
                  onClick={() => refetchPlans()}
                >
                  再試行
                </button>
              </div>
            ) : null}
            <p className="mb-4 text-sm text-muted-foreground">
              今日の学習時間：
              {/* 取得前は 0 分になる。まだ分からないことを 0 と言い切らない。 */}
              <span className="font-bold text-foreground">
                {logsLoading ? "—" : formatMinutes(todayMinutes)}
              </span>
            </p>
            <StudyHeatmap
              ref={heatmapRef}
              plans={plans}
              today={today}
              readOnly={readOnly}
            />
          </CardContent>
        </Card>
      </section>

      <section className="mb-8">
        <h2 className="text-xl font-bold mb-3">直近7日間の科目別学習時間</h2>
        <Card>
          <CardContent className="py-5">
            <SubjectMinutesBars data={subjectMinutes} />
          </CardContent>
        </Card>
      </section>
    </>
  );
}
