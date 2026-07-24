"use client";

import { useGoals, type Goal } from "@/app/hooks/useGoals";
import { buildExamSchedule } from "@/lib/examSchedule";
import { daysUntil, formatExamDate } from "@/lib/date";
import { Badge } from "@/components/ui/badge";
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
          <p className="text-sm text-muted-foreground">
            受験校を決めると、入試日程がここに受験日順で並びます。
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="py-5">
        <ol className="relative space-y-4 pl-5">
          {schedule.map((entry, index) => {
            const days = daysUntil(entry.examDate);
            return (
              <li key={entry.id} className="relative">
                {index < schedule.length - 1 && (
                  <span
                    aria-hidden="true"
                    className="absolute -left-[21.5px] top-2.5 h-[calc(100%+1rem)] w-px bg-border"
                  />
                )}
                {/* タイムラインの節点 */}
                <span
                  aria-hidden="true"
                  className={`absolute -left-[27px] top-1 h-3 w-3 rounded-full border-2 border-background ${
                    entry.isFirstChoice ? "bg-primary" : "bg-muted-foreground"
                  }`}
                />
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <span className="font-medium">
                    {formatExamDate(entry.examDate)}
                  </span>
                  {days >= 0 && (
                    <span className="text-xs text-primary">あと{days}日</span>
                  )}
                  {entry.isFirstChoice && (
                    <Badge variant="info">第一志望</Badge>
                  )}
                  {entry.sameDayCount > 0 && (
                    <Badge variant="destructive">
                      同日に{entry.sameDayCount + 1}校
                    </Badge>
                  )}
                  {entry.sameDayCount === 0 && entry.backToBack && (
                    <Badge variant="warning">連戦</Badge>
                  )}
                </div>
                <p className="text-sm text-foreground">
                  {entry.universityName} {entry.facultyName}
                </p>
                {entry.tags.length > 0 && (
                  <div className="mt-0.5 flex flex-wrap gap-1">
                    {entry.tags.map((tag) => (
                      <Badge key={tag} variant="secondary">
                        #{tag}
                      </Badge>
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
