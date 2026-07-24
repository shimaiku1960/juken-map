"use client";

import { useStudyLogs, type StudyLog } from "@/app/hooks/useStudyLogs";
import { useStudyPlans, type StudyPlan } from "@/app/hooks/useStudyPlans";
import {
  computeStreak,
  computeSubjectMinutes,
} from "@/lib/studyStats";
import { ymdLocal, todayYmd, ymdAfterDays } from "@/lib/date";
import { formatMinutes } from "@/lib/studyLog";
import StreakBadge from "@/app/components/StreakBadge";
import StudyHeatmap from "@/app/components/StudyHeatmap";
import SubjectMinutesBars from "@/app/components/SubjectMinutesBars";
import { Card, CardContent } from "@/components/ui/card";

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
    isError: plansError,
    refetch: refetchPlans,
  } = useStudyPlans(initialPlans);

  const today = todayYmd();
  const streak = computeStreak(logs, today);

  const todayMinutes = logs
    .filter((l) => ymdLocal(l.date) === today)
    .reduce((sum, l) => sum + l.minutes, 0);

  // 直近7日間（今日を含む）の科目別合計
  const weekFrom = ymdAfterDays(-6);
  const subjectMinutes = computeSubjectMinutes(logs, weekFrom, today);

  return (
    <>
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
