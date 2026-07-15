"use client";

import { useState } from "react";
import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import TodayProgressRing from "@/app/components/TodayProgressRing";
import QuickStudyLogDialog from "@/app/components/QuickStudyLogDialog";

export type TodayPlan = {
  id: number;
  content: string;
  done: boolean;
  subject: string | null;
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
  const [currentPlans, setCurrentPlans] = useState(plans);
  const [selectedPlan, setSelectedPlan] = useState<TodayPlan | null>(null);
  const doneCount = currentPlans.filter((p) => p.done).length;

  const handleRecorded = (planId: number, minutes: number) => {
    setCurrentPlans((current) =>
      current.map((plan) =>
        plan.id === planId
          ? { ...plan, done: true, recordedMinutes: minutes }
          : plan
      )
    );
  };

  return (
    <Card>
      <CardContent className="py-5">
        {currentPlans.length === 0 ? (
          <div className="space-y-3">
            <p className="text-muted-foreground">今日の学習予定はありません。</p>
            <div className="flex flex-col gap-2 sm:flex-row">
              <Button asChild>
                <Link href="/schedule">今日の予定を作る</Link>
              </Button>
              <Button asChild variant="outline">
                <a href="#manual-study-log">予定外の学習を記録</a>
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:gap-5">
            <div className="shrink-0">
              <TodayProgressRing done={doneCount} total={currentPlans.length} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="mb-2 text-sm font-medium text-gray-600">
                達成 {doneCount} / {currentPlans.length}
              </p>
              <ul className="space-y-3">
                {currentPlans.map((plan) => (
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
                    {plan.recordedMinutes == null && (
                      <Button
                        type="button"
                        variant={plan.done ? "outline" : "default"}
                        className="h-11 w-full sm:w-auto"
                        onClick={() => setSelectedPlan(plan)}
                      >
                        {plan.done ? "実績を追加" : "実績を記録"}
                      </Button>
                    )}
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
      <QuickStudyLogDialog
        plan={selectedPlan}
        open={selectedPlan !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedPlan(null);
        }}
        onSuccess={handleRecorded}
      />
    </Card>
  );
}
