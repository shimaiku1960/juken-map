"use client";

import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import TodayProgressRing from "@/app/components/TodayProgressRing";

export type TodayPlan = {
  id: number;
  content: string;
  done: boolean;
  subject: string | null;
  textbookId: number | null;
  textbookName: string | null;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  recordedMinutes: number | null;
};

export default function TodayStudyPlans({
  plans,
  weekCount,
}: {
  plans: TodayPlan[];
  weekCount: number;
}) {
  const doneCount = plans.filter((plan) => plan.done).length;

  return (
    <Card>
      <CardContent className="py-5">
        {plans.length === 0 ? (
          <div className="space-y-3">
            <p className="text-muted-foreground">今日の学習予定はありません。</p>
            <Button asChild variant="outline">
              <Link href="/schedule">今日の予定を作る</Link>
            </Button>
          </div>
        ) : (
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:gap-5">
            <div className="shrink-0">
              <TodayProgressRing done={doneCount} total={plans.length} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="mb-2 text-sm font-medium text-gray-600">
                達成 {doneCount} / {plans.length}
              </p>
              <ul className="space-y-3">
                {plans.map((plan) => (
                  <li
                    key={plan.id}
                    className="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center"
                  >
                    <div className="min-w-0 flex-1">
                      <p className={plan.done ? "text-muted-foreground line-through" : "font-medium"}>
                        {plan.content}
                      </p>
                      {plan.recordedMinutes != null ? (
                        <p className="mt-1 text-sm text-muted-foreground">
                          記録済み {plan.recordedMinutes}分
                        </p>
                      ) : plan.done ? (
                        <p className="mt-1 text-sm text-amber-700">
                          完了・実績未記録
                        </p>
                      ) : null}
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        )}
        <div className="mt-3 flex items-center justify-between text-sm text-gray-500">
          <span>今後7日間の予定：{weekCount} 件</span>
          <Link href="/schedule" className="text-blue-600 hover:underline">
            学習予定カレンダー →
          </Link>
        </div>
      </CardContent>
    </Card>
  );
}
