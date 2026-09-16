import { useState } from "react";
import { toast } from "sonner";
import type { StudyPlan } from "@/web/hooks/useStudyPlans";
import {
  useDeleteStudyPlan,
  useUpdateStudyPlan,
} from "@/web/hooks/useStudyPlans";
import StudyPlanCreateDialog from "@/web/components/StudyPlanCreateDialog";
import StudyPlanEditDialog from "@/web/components/StudyPlanEditDialog";
import { studyPlanLabel } from "@/web/lib/studyPlan";
import { subjectColor, subjectLabel } from "@/shared/subjects";
import { Button } from "@/web/components/ui/button";
import { notifyDemoReadOnly } from "@/web/lib/demo-client";

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
  const [adding, setAdding] = useState(false);
  const [editingPlan, setEditingPlan] = useState<StudyPlan | null>(null);

  // 通信と一覧の再取得はフック側。ここでは結果の知らせ方だけを決める。
  // 完了の切り替えと編集ダイアログの保存で同じ知らせ方をするため、
  // コールバックは1つにまとめて両方から渡す。
  const updatePlan = useUpdateStudyPlan();
  const deletePlan = useDeleteStudyPlan();

  const updateCallbacks = {
    onSuccess: () => {
      toast.success("学習予定を更新しました");
      setEditingPlan(null);
    },
    onError: (error: Error) => toast.error(error.message),
  };

  const openEdit = (plan: StudyPlan) => {
    updatePlan.reset();
    setEditingPlan(plan);
  };

  const toggleDone = (plan: StudyPlan) => {
    updatePlan.mutate({ id: plan.id, data: { done: !plan.done } }, updateCallbacks);
  };

  return (
    <section aria-labelledby="selected-day-plans">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h4 id="selected-day-plans" className="text-sm font-semibold text-foreground">
          学習予定 {plans.length > 0 ? `${plans.length}件` : ""}
        </h4>
        {allowAdd ? (
          <Button
            type="button"
            size="lg"
            variant="outline"
            className="h-11"
            title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
            onClick={readOnly ? notifyDemoReadOnly : () => setAdding(true)}
          >
            ＋ 予定を追加
          </Button>
        ) : null}
      </div>

      {plans.length === 0 ? (
        <p className="rounded-lg bg-info/10 px-3 py-4 text-sm text-info">
          この日の予定はまだありません。
        </p>
      ) : (
        <ul className="space-y-2">
          {plans.map((plan) => (
            <li key={plan.id} className="rounded-lg border px-3 py-3">
              <div className="flex items-start gap-3">
                <button
                  type="button"
                  role="checkbox"
                  aria-checked={plan.done}
                  aria-label={`${studyPlanLabel(plan)}を${plan.done ? "未完了に戻す" : "完了にする"}`}
                  disabled={plan.studyLogId != null || updatePlan.isPending}
                  title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
                  onClick={readOnly ? notifyDemoReadOnly : () => toggleDone(plan)}
                  className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                    plan.done
                      ? "border-success bg-success text-success-foreground"
                      : "border-border bg-card"
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
                    <span className="text-xs text-muted-foreground">
                      {subjectLabel(plan.subject)}
                    </span>
                  </div>
                  <p
                    className={`mt-1 text-sm font-medium ${
                      plan.done ? "text-muted-foreground line-through" : "text-foreground"
                    }`}
                  >
                    {studyPlanLabel(plan)}
                  </p>
                  {plan.studyLogId != null ? (
                    <p className="mt-1 text-xs text-success">実績記録済み</p>
                  ) : null}
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
                    onClick={
                      readOnly ? notifyDemoReadOnly : () => openEdit(plan)
                    }
                  >
                    編集
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    disabled={deletePlan.isPending}
                    title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
                    onClick={() => {
                      if (readOnly) {
                        notifyDemoReadOnly();
                        return;
                      }
                      if (window.confirm("この学習予定を削除しますか？")) {
                        deletePlan.mutate(plan.id, {
                          onSuccess: () =>
                            toast.success("学習予定を削除しました"),
                          // 削除は元々サーバーの文言を読んでおらず、常にこの
                          // 一文を出していた。表示を変えないためここで固定する。
                          onError: () =>
                            toast.error("予定の削除に失敗しました"),
                        });
                      }
                    }}
                  >
                    削除
                  </Button>
                </div>
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
            updatePlan.mutate({ id: editingPlan.id, data }, updateCallbacks);
          }}
          onClose={() => setEditingPlan(null)}
        />
      ) : null}
    </section>
  );
}
