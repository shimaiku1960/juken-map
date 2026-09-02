"use client";

import { useRef } from "react";
import { useStudyLogs, type StudyLog } from "@/app/hooks/useStudyLogs";
import { useStudyPlans, type StudyPlan } from "@/app/hooks/useStudyPlans";
import {
  computeStreak,
  computeSubjectMinutes,
} from "@/lib/studyStats";
import { ymdLocal, todayYmd, ymdAfterDays } from "@/lib/date";
import { formatMinutes } from "@/lib/studyLog";
import StreakBadge from "@/app/components/StreakBadge";
import StudyHeatmap, { type StudyHeatmapHandle } from "@/app/components/StudyHeatmap";
import SubjectMinutesBars from "@/app/components/SubjectMinutesBars";
import TodayStudyPlans from "@/app/components/TodayStudyPlans";
import { Card, CardContent } from "@/components/ui/card";
import { studyPlanLabel } from "@/lib/studyPlan";

// 実績（StudyLog）まわりのダッシュボード。記録するとキャッシュ更新で
// ストリーク・ヒートマップ・科目別バーが即座に伸びる（クライアントで集計）。
export default function StudyRecordDashboard({
  initialLogs,
  initialPlans,
  readOnly,
}: {
  initialLogs: StudyLog[];
  initialPlans: StudyPlan[];
  readOnly: boolean;
}) {
  const { data: logs = [] } = useStudyLogs(initialLogs);
  const {
    data: plans = [],
    isPending: plansLoading,
    isError: plansError,
    refetch: refetchPlans,
  } = useStudyPlans(initialPlans);

  const today = todayYmd();
  const heatmapRef = useRef<StudyHeatmapHandle>(null);
  const streak = computeStreak(logs, today);

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
  const weekFrom = ymdAfterDays(-6);
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
              <span className="font-bold text-foreground">
                {formatMinutes(todayMinutes)}
              </span>
            </p>
            <StudyHeatmap
              ref={heatmapRef}
              logs={logs}
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
