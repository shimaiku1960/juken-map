"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { StudyPlan } from "@/app/hooks/useStudyPlans";
import { studyPlansKey } from "@/app/hooks/useStudyPlans";
import type { UpdateStudyPlanInput } from "@/lib/validations/studyPlan";
import StudyPlanCreateDialog from "@/app/components/StudyPlanCreateDialog";
import StudyPlanEditDialog from "@/app/components/StudyPlanEditDialog";
import { studyPlanLabel } from "@/lib/studyPlan";
import { subjectColor, subjectLabel } from "@/lib/subjects";
import { Button } from "@/components/ui/button";

export default function StudyDayPlanPanel({
  date,
  plans,
  readOnly,
  allowAdd = true,
}: {
  date: string;
  plans: StudyPlan[];
  readOnly: boolean;
  allowAdd?: boolean;
}) {
  const queryClient = useQueryClient();
  const [adding, setAdding] = useState(false);
  const [editingPlan, setEditingPlan] = useState<StudyPlan | null>(null);

  const updatePlan = useMutation({
    mutationFn: async ({ id, data }: { id: number; data: UpdateStudyPlanInput }) => {
      const response = await fetch(`/api/study-plans/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => null);
        const issueMessage = Array.isArray(body?.error)
          ? body.error.find((issue: unknown) =>
              typeof issue === "object" && issue !== null && "message" in issue
            )?.message
          : null;
        throw new Error(
          typeof body?.error === "string"
            ? body.error
            : typeof issueMessage === "string"
              ? issueMessage
              : "予定の更新に失敗しました"
        );
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: studyPlansKey });
      toast.success("学習予定を更新しました");
      setEditingPlan(null);
    },
    onError: (error) => toast.error(error.message),
  });

  const deletePlan = useMutation({
    mutationFn: async (id: number) => {
      const response = await fetch(`/api/study-plans/${id}`, {
        method: "DELETE",
      });
      if (!response.ok) throw new Error("予定の削除に失敗しました");
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: studyPlansKey });
      toast.success("学習予定を削除しました");
    },
    onError: (error) => toast.error(error.message),
  });

  const openEdit = (plan: StudyPlan) => {
    updatePlan.reset();
    setEditingPlan(plan);
  };

  const toggleDone = (plan: StudyPlan) => {
    updatePlan.mutate({ id: plan.id, data: { done: !plan.done } });
  };

  return (
    <section aria-labelledby="selected-day-plans">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h4 id="selected-day-plans" className="text-sm font-semibold text-gray-700">
          学習予定 {plans.length > 0 ? `${plans.length}件` : ""}
        </h4>
        {!readOnly && allowAdd ? (
          <Button type="button" size="sm" variant="outline" onClick={() => setAdding(true)}>
            ＋ 予定を追加
          </Button>
        ) : null}
      </div>

      {plans.length === 0 ? (
        <p className="rounded-md bg-blue-50 px-3 py-4 text-sm text-blue-900">
          この日の予定はまだありません。
        </p>
      ) : (
        <ul className="space-y-2">
          {plans.map((plan) => (
            <li key={plan.id} className="rounded-md border px-3 py-3">
              <div className="flex items-start gap-3">
                <button
                  type="button"
                  role="checkbox"
                  aria-checked={plan.done}
                  aria-label={`${studyPlanLabel(plan)}を${plan.done ? "未完了に戻す" : "完了にする"}`}
                  disabled={readOnly || plan.studyLogId != null || updatePlan.isPending}
                  onClick={() => toggleDone(plan)}
                  className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 ${
                    plan.done
                      ? "border-green-600 bg-green-600 text-white"
                      : "border-gray-300 bg-white"
                  }`}
                >
                  {plan.done ? "✓" : ""}
                </button>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span
                      className="h-2.5 w-2.5 shrink-0 rounded-full"
                      style={{ backgroundColor: subjectColor(plan.subject) }}
                      aria-hidden="true"
                    />
                    <span className="text-xs text-gray-500">
                      {subjectLabel(plan.subject)}
                    </span>
                  </div>
                  <p
                    className={`mt-1 text-sm font-medium ${
                      plan.done ? "text-gray-500 line-through" : "text-gray-900"
                    }`}
                  >
                    {studyPlanLabel(plan)}
                  </p>
                  {plan.studyLogId != null ? (
                    <p className="mt-1 text-xs text-green-700">実績記録済み</p>
                  ) : null}
                </div>
                {!readOnly ? (
                  <div className="flex shrink-0 gap-1">
                    <Button type="button" size="sm" variant="ghost" onClick={() => openEdit(plan)}>
                      編集
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={deletePlan.isPending}
                      onClick={() => {
                        if (window.confirm("この学習予定を削除しますか？")) {
                          deletePlan.mutate(plan.id);
                        }
                      }}
                    >
                      削除
                    </Button>
                  </div>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}

      {adding ? (
        <StudyPlanCreateDialog date={date} onClose={() => setAdding(false)} />
      ) : null}

      {editingPlan ? (
        <StudyPlanEditDialog
          key={editingPlan.id}
          plan={editingPlan}
          saving={updatePlan.isPending}
          saveError={updatePlan.error?.message ?? null}
          onSave={(data) => {
            updatePlan.reset();
            updatePlan.mutate({ id: editingPlan.id, data });
          }}
          onClose={() => setEditingPlan(null)}
        />
      ) : null}
    </section>
  );
}
