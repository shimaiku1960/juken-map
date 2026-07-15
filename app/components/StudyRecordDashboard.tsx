"use client";

import { useStudyLogs, type StudyLog } from "@/app/hooks/useStudyLogs";
import {
  computeStreak,
  computeHeatmap,
  computeSubjectMinutes,
} from "@/lib/studyStats";
import { ymdLocal, todayYmd, ymdAfterDays } from "@/lib/date";
import { formatMinutes } from "@/lib/studyLog";
import StreakBadge from "@/app/components/StreakBadge";
import StudyHeatmap from "@/app/components/StudyHeatmap";
import SubjectMinutesBars from "@/app/components/SubjectMinutesBars";
import StudyLogForm from "@/app/components/StudyLogForm";
import RecentStudyLogs from "@/app/components/RecentStudyLogs";
import { Card, CardContent } from "@/components/ui/card";

// 実績（StudyLog）まわりのダッシュボード。記録するとキャッシュ更新で
// ストリーク・ヒートマップ・科目別バーが即座に伸びる（クライアントで集計）。
export default function StudyRecordDashboard({
  initialLogs,
}: {
  initialLogs: StudyLog[];
}) {
  const { data: logs = [] } = useStudyLogs(initialLogs);

  const today = todayYmd();
  const streak = computeStreak(logs, today);

  const todayMinutes = logs
    .filter((l) => ymdLocal(l.date) === today)
    .reduce((sum, l) => sum + l.minutes, 0);

  const heatmapWeeks = computeHeatmap(logs, today);
  const monthLabel = new Date().toLocaleDateString("ja-JP", {
    year: "numeric",
    month: "long",
  });

  // 直近7日間（今日を含む）の科目別合計
  const weekFrom = ymdAfterDays(-6);
  const subjectMinutes = computeSubjectMinutes(logs, weekFrom, today);

  return (
    <>
      <section className="mb-8">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-xl font-bold">学習の記録</h2>
          <StreakBadge streak={streak} />
        </div>
        <Card>
          <CardContent className="py-5">
            <p className="mb-4 text-sm text-gray-600">
              今日の学習時間：
              <span className="font-bold text-gray-900">
                {formatMinutes(todayMinutes)}
              </span>
            </p>
            <StudyHeatmap weeks={heatmapWeeks} monthLabel={monthLabel} />
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

      <section id="manual-study-log" className="mb-8 scroll-mt-6">
        <h2 className="text-xl font-bold mb-3">予定外の学習を記録</h2>
        <Card>
          <CardContent className="space-y-5 py-5">
            <StudyLogForm />
            <div className="border-t pt-4">
              <p className="mb-2 text-sm font-medium text-gray-600">
                最近の記録
              </p>
              <RecentStudyLogs logs={logs} />
            </div>
          </CardContent>
        </Card>
      </section>
    </>
  );
}
