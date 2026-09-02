"use client";

import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import TodayProgressRing from "@/app/components/TodayProgressRing";
import { notifyDemoReadOnly } from "@/lib/demo-client";

export type TodayPlan = {
  id: number;
  label: string;
  done: boolean;
  recordedMinutes: number | null;
};

export function todayPlanState(plans: TodayPlan[], hasError: boolean) {
  if (hasError) return "error" as const;
  if (plans.length === 0) return "empty" as const;
  if (plans.every((plan) => plan.done)) return "complete" as const;
  return "active" as const;
}

export default function TodayStudyPlans({
  plans,
  weekCount,
  hasError,
  isLoading,
  readOnly,
  onRetry,
  onShowToday,
}: {
  plans: TodayPlan[];
  weekCount: number;
  hasError: boolean;
  isLoading: boolean;
  readOnly: boolean;
  onRetry: () => void;
  onShowToday: (openCreate: boolean) => void;
}) {
  const sortedPlans = [...plans].sort((a, b) => Number(a.done) - Number(b.done));
  const doneCount = plans.filter((plan) => plan.done).length;
  const state = todayPlanState(plans, hasError);

  return (
    <Card>
      <CardContent className="py-5">
        {isLoading ? (
          <div role="status" aria-label="今日の予定を読み込み中" className="space-y-3">
            <div className="h-5 w-32 animate-pulse rounded bg-muted" />
            <div className="h-12 animate-pulse rounded bg-muted" />
            <div className="h-11 animate-pulse rounded bg-muted" />
          </div>
        ) : (
          <>
            {hasError ? (
              <div role="alert" className="mb-4 flex flex-wrap items-center justify-between gap-2 rounded-lg bg-destructive/10 px-3 py-3 text-sm text-destructive">
                <span>予定を更新できませんでした。表示が古い可能性があります。</span>
                <Button type="button" variant="ghost" size="sm" onClick={onRetry}>
                  再試行
                </Button>
              </div>
            ) : null}

            {plans.length === 0 ? (
              hasError ? null : (
                <p className="text-muted-foreground">
                  今日の学習予定はありません。まず今日やることを決めましょう。
                </p>
              )
            ) : (
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:gap-5">
                <div className="shrink-0">
                  <TodayProgressRing done={doneCount} total={plans.length} />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="mb-2 text-sm font-medium text-muted-foreground">
                    達成 {doneCount} / {plans.length}
                  </p>
                  {state === "complete" ? (
                    <p className="mb-3 font-medium text-success">今日の予定はすべて完了しました。</p>
                  ) : null}
                  <ul className="space-y-2">
                    {sortedPlans.map((plan) => (
                      <li key={plan.id} className="rounded-lg border px-3 py-3">
                        <div className="flex items-start gap-2">
                          <span className="mt-0.5 text-sm text-muted-foreground" aria-hidden="true">
                            {plan.done ? "✓" : "□"}
                          </span>
                          <div className="min-w-0 flex-1">
                            <span className="sr-only">{plan.done ? "完了" : "未完了"}：</span>
                            <p className={plan.done ? "text-sm text-muted-foreground line-through" : "text-sm font-medium"}>
                              {plan.label}
                            </p>
                            {plan.recordedMinutes != null ? (
                              <p className="mt-1 text-xs text-muted-foreground">記録済み {plan.recordedMinutes}分</p>
                            ) : null}
                          </div>
                        </div>
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            )}

            {state === "active" ? (
              readOnly ? (
                <Button type="button" size="lg" className="mt-4 h-11 w-full sm:w-auto" onClick={notifyDemoReadOnly}>
                  学習を始める
                </Button>
              ) : (
                <Button asChild size="lg" className="mt-4 h-11 w-full sm:w-auto">
                  <Link href="/#study-start">学習を始める</Link>
                </Button>
              )
            ) : state === "empty" ? (
              <Button
                type="button"
                size="lg"
                className="mt-4 h-11 w-full sm:w-auto"
                onClick={readOnly ? notifyDemoReadOnly : () => onShowToday(true)}
              >
                今日の予定を作る
              </Button>
            ) : state === "complete" ? (
              <Button
                type="button"
                size="lg"
                className="mt-4 h-11 w-full sm:w-auto"
                onClick={() => onShowToday(false)}
              >
                今日の記録を確認
              </Button>
            ) : null}

            {!hasError ? (
              <div className="mt-4 flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
                <span>今後7日間の予定：{weekCount}件</span>
                <Link href="/dashboard#study-calendar" className="font-medium text-primary hover:underline">
                  学習予定カレンダー →
                </Link>
              </div>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  );
}
