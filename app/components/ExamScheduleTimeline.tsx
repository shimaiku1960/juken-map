"use client";

import { useGoals, type Goal } from "@/app/hooks/useGoals";
import { buildExamSchedule } from "@/lib/examSchedule";
import { daysUntil, formatExamDate } from "@/lib/date";
import { Card, CardContent } from "@/components/ui/card";

// 受験校（decided）の入試日程を受験日順に俯瞰するタイムライン。
// 同日受験・連戦を警告表示し、日程面の受験戦略を一目で把握できるようにする。
export default function ExamScheduleTimeline({
  initialGoals,
}: {
  initialGoals: Goal[];
}) {
  const { data: goals = [] } = useGoals(initialGoals);

  const decided = goals.filter((goal) => goal.status === "decided");
  const schedule = buildExamSchedule(
    decided.map((goal) => ({
      id: goal.id,
      universityName: goal.faculty.university.name,
      facultyName: goal.faculty.name,
      examDate: goal.faculty.examDate,
      isFirstChoice: goal.isFirstChoice,
      tags: goal.faculty.tags.map((tag) => tag.name),
    }))
  );

  if (schedule.length === 0) {
    return (
      <Card>
        <CardContent className="py-5">
          <p className="text-sm text-gray-400">
            受験校を決めると、入試日程がここに受験日順で並びます。
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="py-5">
        <ol className="relative space-y-4 border-l border-gray-200 pl-5">
          {schedule.map((entry) => {
            const days = daysUntil(entry.examDate);
            return (
              <li key={entry.id} className="relative">
                {/* タイムラインの節点 */}
                <span
                  className={`absolute -left-[27px] top-1 h-3 w-3 rounded-full border-2 border-white ${
                    entry.isFirstChoice ? "bg-blue-600" : "bg-gray-400"
                  }`}
                />
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <span className="font-medium">
                    {formatExamDate(entry.examDate)}
                  </span>
                  {days >= 0 && (
                    <span className="text-xs text-blue-600">あと{days}日</span>
                  )}
                  {entry.isFirstChoice && (
                    <span className="rounded bg-blue-50 px-1.5 py-0.5 text-xs text-blue-700">
                      第一志望
                    </span>
                  )}
                  {entry.sameDayCount > 0 && (
                    <span className="rounded bg-red-50 px-1.5 py-0.5 text-xs text-red-600">
                      同日に{entry.sameDayCount + 1}校
                    </span>
                  )}
                  {entry.sameDayCount === 0 && entry.backToBack && (
                    <span className="rounded bg-amber-50 px-1.5 py-0.5 text-xs text-amber-700">
                      連戦
                    </span>
                  )}
                </div>
                <p className="text-sm text-gray-700">
                  {entry.universityName} {entry.facultyName}
                </p>
                {entry.tags.length > 0 && (
                  <div className="mt-0.5 flex flex-wrap gap-1">
                    {entry.tags.map((tag) => (
                      <span
                        key={tag}
                        className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600"
                      >
                        #{tag}
                      </span>
                    ))}
                  </div>
                )}
              </li>
            );
          })}
        </ol>
      </CardContent>
    </Card>
  );
}
