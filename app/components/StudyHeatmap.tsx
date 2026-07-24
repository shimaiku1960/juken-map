"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  type StudyLog,
  useDeleteStudyLog,
} from "@/app/hooks/useStudyLogs";
import type { StudyPlan } from "@/app/hooks/useStudyPlans";
import StudyDayPlanPanel from "@/app/components/StudyDayPlanPanel";
import QuickManualStudyLogDialog from "@/app/components/QuickManualStudyLogDialog";
import { computeHeatmap } from "@/lib/studyStats";
import { formatMinutes, formatStudyRange } from "@/lib/studyLog";
import { subjectColor, subjectLabel } from "@/lib/subjects";
import { ymdLocal } from "@/lib/date";
import { Button } from "@/components/ui/button";

function levelClass(minutes: number): string {
  if (minutes <= 0) return "bg-muted";
  if (minutes < 30) return "bg-success/20";
  if (minutes < 60) return "bg-success/55";
  if (minutes < 120) return "bg-success/75";
  return "bg-success";
}

const WEEKDAYS = ["日", "月", "火", "水", "木", "金", "土"];

function compactMinutes(minutes: number): string {
  if (minutes < 60) return `${minutes}分`;
  const hours = Math.floor(minutes / 60);
  const remainder = minutes % 60;
  return remainder === 0 ? `${hours}h` : `${hours}h${remainder}`;
}

function dateLabel(ymd: string): string {
  const [, month, day] = ymd.split("-").map(Number);
  return `${month}月${day}日`;
}

function monthLabel(monthYmd: string): string {
  const [year, month] = monthYmd.split("-").map(Number);
  return `${year}年${month}月`;
}

function shiftMonth(monthYmd: string, amount: number): string {
  const [year, month] = monthYmd.split("-").map(Number);
  const date = new Date(year, month - 1 + amount, 1);
  const nextYear = date.getFullYear();
  const nextMonth = String(date.getMonth() + 1).padStart(2, "0");
  return `${nextYear}-${nextMonth}-01`;
}

function planDate(plan: StudyPlan): string {
  return plan.date.slice(0, 10);
}

export default function StudyHeatmap({
  logs,
  plans,
  today,
  readOnly,
}: {
  logs: StudyLog[];
  plans: StudyPlan[];
  today: string;
  readOnly: boolean;
}) {
  const deleteLog = useDeleteStudyLog();
  const currentMonth = `${today.slice(0, 7)}-01`;
  const [displayMonth, setDisplayMonth] = useState(currentMonth);
  const [selectedYmd, setSelectedYmd] = useState(today);
  const [recordDate, setRecordDate] = useState<string | null>(null);
  const weeks = computeHeatmap(logs, displayMonth);
  const monthPlans = plans.filter((plan) =>
    planDate(plan).startsWith(displayMonth.slice(0, 7))
  );
  const plansByDate = new Map<string, StudyPlan[]>();
  for (const plan of monthPlans) {
    const date = planDate(plan);
    const dayPlans = plansByDate.get(date) ?? [];
    dayPlans.push(plan);
    plansByDate.set(date, dayPlans);
  }
  const selectedPlans = plans.filter((plan) => planDate(plan) === selectedYmd);
  const selectedLogs = logs.filter((log) => ymdLocal(log.date) === selectedYmd);
  const selectedMinutes = selectedLogs.reduce(
    (sum, log) => sum + log.minutes,
    0
  );
  const visibleSubjects = Array.from(
    new Set([
      ...weeks
        .flat()
        .flatMap((cell) => cell.subjects.map((subject) => subject.value)),
      ...monthPlans.map((plan) => plan.subject ?? "other"),
    ])
  );

  const moveMonth = (amount: number) => {
    const next = shiftMonth(displayMonth, amount);
    setDisplayMonth(next);
    setSelectedYmd(next === currentMonth ? today : next);
  };

  const returnToToday = () => {
    setDisplayMonth(currentMonth);
    setSelectedYmd(today);
  };

  const selectedIsPast = selectedYmd < today;
  const selectedIsToday = selectedYmd === today;
  const detailHeading = selectedIsPast
    ? selectedPlans.length > 0
      ? `${dateLabel(selectedYmd)}の予定と実績`
      : `${dateLabel(selectedYmd)}の学習実績`
    : selectedIsToday
      ? "今日の予定と実績"
      : `${dateLabel(selectedYmd)}の学習予定`;

  return (
    <div>
      <p className="mb-3 text-sm text-muted-foreground">
        日付を選ぶと、その日の予定や学習実績を記録できます。
      </p>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-1">
          <Button type="button" size="sm" variant="ghost" onClick={() => moveMonth(-1)} aria-label="前の月">
            ←
          </Button>
          <p className="min-w-28 text-center text-sm font-semibold text-foreground">
            {monthLabel(displayMonth)}
          </p>
          <Button type="button" size="sm" variant="ghost" onClick={() => moveMonth(1)} aria-label="次の月">
            →
          </Button>
        </div>
        {displayMonth !== currentMonth || selectedYmd !== today ? (
          <Button type="button" size="sm" variant="outline" onClick={returnToToday}>
            今日
          </Button>
        ) : null}
      </div>

      <div className="overflow-x-auto pb-1">
        <div className="min-w-80">
          <div className="mb-0.5 grid grid-cols-7 gap-0.5">
            {WEEKDAYS.map((weekday) => (
              <div key={weekday} className="text-center text-xs text-muted-foreground">
                {weekday}
              </div>
            ))}
          </div>

          <div className="space-y-0.5">
            {weeks.map((week, weekIndex) => (
              <div key={weekIndex} className="grid grid-cols-7 gap-0.5">
                {week.map((cell, cellIndex) => {
                  if (cell.ymd === null) {
                    return <div key={cellIndex} className="aspect-square" />;
                  }

                  const dayPlans = plansByDate.get(cell.ymd) ?? [];
                  const isFuture = cell.ymd > today;
                  const isToday = cell.ymd === today;
                  const selected = selectedYmd === cell.ymd;
                  const planSubjects = Array.from(
                    new Set(dayPlans.map((plan) => plan.subject ?? "other"))
                  );
                  const ariaParts = [
                    cell.minutes > 0 ? formatMinutes(cell.minutes) : "実績なし",
                    ...cell.subjects.map(
                      (subject) =>
                        `${subjectLabel(subject.value)}${formatMinutes(subject.minutes)}`
                    ),
                    dayPlans.length > 0 ? `予定${dayPlans.length}件` : "予定なし",
                  ];

                  return (
                    <button
                      key={cellIndex}
                      type="button"
                      aria-label={`${cell.ymd}、${ariaParts.join("、")}。詳細を表示`}
                      aria-pressed={selected}
                      onClick={() => setSelectedYmd(cell.ymd as string)}
                      className={`flex aspect-square min-w-0 flex-col justify-between rounded-sm border p-1 text-left text-[10px] transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ${
                        isFuture && cell.minutes === 0
                          ? dayPlans.length > 0
                            ? "border-dashed border-info/40 bg-info/10 text-info"
                            : "border-border bg-card text-muted-foreground"
                          : `${levelClass(cell.minutes)} border-transparent ${
                              cell.minutes >= 120 ? "text-success-foreground" : "text-foreground"
                            }`
                      } ${
                        isToday ? "ring-2 ring-ring ring-inset" : ""
                      } ${
                        selected
                          ? "outline-2 outline-offset-1 outline-primary"
                          : "hover:border-muted-foreground"
                      }`}
                    >
                      <span className="flex w-full items-center justify-between gap-1">
                        <span>{Number(cell.ymd.slice(-2))}</span>
                        {isToday ? (
                          <span className="rounded bg-info px-1 text-[8px] text-info-foreground">
                            今日
                          </span>
                        ) : null}
                      </span>
                      <span className="self-center text-[9px] font-semibold tabular-nums sm:text-[10px]">
                        {cell.minutes > 0 ? compactMinutes(cell.minutes) : ""}
                      </span>
                      <span
                        className="flex h-1.5 w-full overflow-hidden rounded-full bg-card/70"
                        aria-hidden="true"
                      >
                        {cell.minutes > 0
                          ? cell.subjects.map((subject) => (
                              <span
                                key={subject.value}
                                style={{
                                  width: `${(subject.minutes / cell.minutes) * 100}%`,
                                  backgroundColor: subjectColor(subject.value),
                                }}
                              />
                            ))
                          : planSubjects.map((subject) => (
                              <span
                                key={subject}
                                style={{
                                  width: `${100 / planSubjects.length}%`,
                                  backgroundColor: subjectColor(subject),
                                }}
                              />
                            ))}
                      </span>
                    </button>
                  );
                })}
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center gap-1">
            <span className="h-3 w-3 rounded-sm bg-success/75" /> 学習時間
          </span>
          <span className="inline-flex items-center gap-1">
            <span className="h-3 w-3 rounded-sm border border-dashed border-primary/60 bg-info/10" /> 予定
          </span>
        </div>
        <div className="flex items-center gap-1" aria-label="実績量の凡例、少ないから多い">
          <span>少</span>
          <span className="h-3 w-3 rounded-sm bg-muted" />
          <span className="h-3 w-3 rounded-sm bg-success/20" />
          <span className="h-3 w-3 rounded-sm bg-success/55" />
          <span className="h-3 w-3 rounded-sm bg-success/75" />
          <span className="h-3 w-3 rounded-sm bg-success" />
          <span>多</span>
        </div>
      </div>

      {visibleSubjects.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {visibleSubjects.map((subject) => (
            <span key={subject} className="inline-flex items-center gap-1">
              <span
                className="h-2 w-2 rounded-full"
                style={{ backgroundColor: subjectColor(subject) }}
                aria-hidden="true"
              />
              {subjectLabel(subject)}
            </span>
          ))}
        </div>
      ) : null}

      <div className="mt-5 space-y-5 border-t pt-4">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h3 className="font-semibold text-foreground" aria-live="polite">
            {detailHeading}
          </h3>
          {selectedMinutes > 0 ? (
            <p className="text-sm text-muted-foreground">
              合計 <span className="font-semibold text-foreground">{formatMinutes(selectedMinutes)}</span>
            </p>
          ) : null}
        </div>

        {!selectedIsPast || selectedPlans.length > 0 ? (
          <StudyDayPlanPanel
            date={selectedYmd}
            plans={selectedPlans}
            readOnly={readOnly}
            allowAdd={!selectedIsPast}
          />
        ) : null}

        {selectedYmd <= today ? (
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h4 className="text-sm font-semibold text-foreground">学習実績</h4>
            {!readOnly ? (
              <Button type="button" size="sm" onClick={() => setRecordDate(selectedYmd)}>
                {selectedIsToday ? "今日の学習時間を記録" : "この日の学習時間を記録"}
              </Button>
            ) : null}
          </div>
        ) : null}

        {selectedLogs.length === 0 ? (
          selectedIsPast || selectedIsToday ? (
            <div className="rounded-md bg-muted/50 px-3 py-4 text-sm text-muted-foreground">
              <p>この日の学習時間はまだ記録されていません。</p>
            </div>
          ) : null
        ) : (
          <ul className="space-y-2">
            {selectedLogs.map((log) => {
              const range = formatStudyRange(
                log.rangeStart,
                log.rangeEnd,
                log.rangeUnit
              );
              return (
                <li key={log.id} className="rounded-md border px-3 py-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex min-w-0 items-center gap-2">
                      <span
                        className="h-2.5 w-2.5 shrink-0 rounded-full"
                        style={{ backgroundColor: subjectColor(log.subject) }}
                        aria-hidden="true"
                      />
                      <span className="font-medium text-foreground">
                        {subjectLabel(log.subject)}
                      </span>
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <span className="text-sm font-semibold tabular-nums text-foreground">
                        {formatMinutes(log.minutes)}
                      </span>
                      {!readOnly ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="min-h-11 text-destructive hover:bg-destructive/10 hover:text-destructive"
                          disabled={deleteLog.isPending}
                          aria-label={`${dateLabel(selectedYmd)}の${subjectLabel(log.subject)}の実績を削除`}
                          onClick={() => {
                            if (
                              !window.confirm(
                                `${dateLabel(selectedYmd)}の${subjectLabel(log.subject)}（${formatMinutes(log.minutes)}）を削除しますか？`
                              )
                            ) {
                              return;
                            }
                            deleteLog.mutate(log.id, {
                              onSuccess: () => toast.success("学習実績を削除しました"),
                              onError: (error) => toast.error(error.message),
                            });
                          }}
                        >
                          削除
                        </Button>
                      ) : null}
                    </div>
                  </div>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {log.textbook?.name || log.memo?.trim() || "教材・内容の記録なし"}
                  </p>
                  {range ? (
                    <p className="mt-1 text-xs text-muted-foreground">学習範囲：{range}</p>
                  ) : null}
                  {log.textbook && log.memo?.trim() ? (
                    <p className="mt-1 text-xs text-muted-foreground">{log.memo.trim()}</p>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <QuickManualStudyLogDialog
        open={recordDate !== null}
        initialDate={recordDate ?? undefined}
        onOpenChange={(open) => {
          if (!open) setRecordDate(null);
        }}
      />
    </div>
  );
}
