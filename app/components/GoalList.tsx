"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { daysUntil, formatExamDate } from "@/lib/date";
import { useGoals, goalsKey, type Goal } from "@/app/hooks/useGoals";
import { notifyDemoReadOnly } from "@/lib/demo-client";

type Props = {
  initialGoals: Goal[];
  readOnly?: boolean;
};

export default function GoalList({
  initialGoals,
  readOnly = false,
}: Props) {
  const queryClient = useQueryClient();

  // サーバー状態の取得。SSR で渡された initialGoals を初期キャッシュとして使う
  const { data: goals = [] } = useGoals(initialGoals);

  // 削除
  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      const res = await fetch(`/api/goals/${id}`, { method: "DELETE" });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "削除に失敗しました");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
      toast.success("目標を削除しました");
    },
    onError: (error) => toast.error(error.message),
  });

  // 第一志望の設定/解除
  const firstChoiceMutation = useMutation({
    mutationFn: async ({ id, value }: { id: number; value: boolean }) => {
      const res = await fetch(`/api/goals/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ isFirstChoice: value }),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "更新に失敗しました");
      }
      return value;
    },
    onSuccess: (value) => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
      toast.success(value ? "第一志望に設定しました" : "第一志望を解除しました");
    },
    onError: (error) => toast.error(error.message),
  });

  // 候補 → 受験校に確定
  const statusMutation = useMutation({
    mutationFn: async (id: number) => {
      const res = await fetch(`/api/goals/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: "decided" }),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "更新に失敗しました");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
      toast.success("受験校に決めました");
    },
    onError: (error) => toast.error(error.message),
  });

  const runIfWritable = (action: () => void) => {
    if (readOnly) {
      notifyDemoReadOnly();
      return;
    }
    action();
  };

  const decided = goals.filter((goal) => goal.status === "decided");
  const candidates = goals.filter((goal) => goal.status === "candidate");
  // 比較テーブルは受験日の早い順に並べる（日程の比較をしやすく）
  const sortedCandidates = [...candidates].sort(
    (a, b) =>
      new Date(a.faculty.examDate).getTime() -
      new Date(b.faculty.examDate).getTime()
  );
  const firstChoice = decided.find((goal) => goal.isFirstChoice);
  const others = decided.filter((goal) => !goal.isFirstChoice);

  // 受験校（decided）カード。候補は比較テーブルで別途表示する。
  const renderGoalCard = (goal: Goal) => {
    const days = daysUntil(goal.faculty.examDate);
    return (
      <li
        key={goal.id}
        className="border rounded px-4 py-3 flex justify-between items-start"
      >
        <div className="space-y-1">
          <p className="font-medium">
            {goal.faculty.university.name} {goal.faculty.name}
          </p>
          <p className="text-sm text-muted-foreground">
            受験日 {formatExamDate(goal.faculty.examDate)}
            {days >= 0 && (
              <span className="ml-2 text-primary">あと{days}日</span>
            )}
          </p>
          <div className="flex flex-wrap gap-1">
            {goal.faculty.tags.map((tag) => (
              <span
                key={tag.name}
                className="text-xs bg-muted text-muted-foreground rounded px-2 py-0.5"
              >
                #{tag.name}
              </span>
            ))}
          </div>
        </div>
        <div className="flex items-center gap-3 shrink-0">
          <button
            title={
              readOnly
                ? "デモアカウントは閲覧専用です"
                : goal.isFirstChoice
                  ? "第一志望を解除"
                  : "第一志望にする"
            }
            onClick={() =>
              runIfWritable(() =>
                firstChoiceMutation.mutate({
                  id: goal.id,
                  value: !goal.isFirstChoice,
                })
              )
            }
            className="text-sm hover:underline"
          >
            {goal.isFirstChoice ? "★ 第一志望" : "☆ 第一志望にする"}
          </button>
          <button
            title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
            onClick={() => runIfWritable(() => deleteMutation.mutate(goal.id))}
            className="text-destructive text-sm hover:underline"
          >
            削除
          </button>
        </div>
      </li>
    );
  };

  return (
    <div>
      <section className="mb-6">
        <h3 className="text-sm font-bold text-muted-foreground mb-2">第一志望</h3>
        {firstChoice ? (
          <ul>{renderGoalCard(firstChoice)}</ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            まだ設定されていません（併願校から ☆ で設定できます）
          </p>
        )}
      </section>

      <section className="mb-6">
        <h3 className="text-sm font-bold text-muted-foreground mb-2">併願校</h3>
        {others.length > 0 ? (
          <ul className="space-y-2">
            {others.map((goal) => renderGoalCard(goal))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">併願校はありません</p>
        )}
      </section>

      <section>
        <h3 className="text-sm font-bold text-muted-foreground mb-2">
          検討中（候補）
        </h3>
        {candidates.length > 0 ? (
          <>
            <p className="mb-2 text-xs text-muted-foreground">
              受験日順に並べて比較できます。決めたら「受験校にする」を押してください。
            </p>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[520px] text-sm">
                <thead>
                  <tr className="border-b text-left text-xs text-muted-foreground">
                    <th className="py-2 pr-3 font-medium">大学・学部</th>
                    <th className="py-2 pr-3 font-medium">系統</th>
                    <th className="py-2 pr-3 font-medium">受験日</th>
                    <th className="py-2 pr-3 font-medium">区分</th>
                    <th className="py-2 font-medium" />
                  </tr>
                </thead>
                <tbody>
                  {sortedCandidates.map((goal) => {
                    const days = daysUntil(goal.faculty.examDate);
                    return (
                      <tr key={goal.id} className="border-b align-top">
                        <td className="py-3 pr-3">
                          <p className="font-medium">
                            {goal.faculty.university.name}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {goal.faculty.name}
                          </p>
                        </td>
                        <td className="py-3 pr-3">
                          <div className="flex flex-wrap gap-1">
                            {goal.faculty.tags.map((tag) => (
                              <span
                                key={tag.name}
                                className="rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground"
                              >
                                #{tag.name}
                              </span>
                            ))}
                          </div>
                        </td>
                        <td className="py-3 pr-3 whitespace-nowrap">
                          {formatExamDate(goal.faculty.examDate)}
                          {days >= 0 && (
                            <span className="ml-1 text-xs text-primary">
                              (あと{days}日)
                            </span>
                          )}
                        </td>
                        <td className="py-3 pr-3 whitespace-nowrap text-muted-foreground">
                          {goal.faculty.university.type || "—"}
                        </td>
                        <td className="py-3 whitespace-nowrap text-right">
                          <button
                            title={
                              readOnly
                                ? "デモアカウントは閲覧専用です"
                                : undefined
                            }
                            onClick={() =>
                              runIfWritable(() =>
                                statusMutation.mutate(goal.id)
                              )
                            }
                            disabled={statusMutation.isPending}
                            className="text-sm text-primary hover:underline"
                          >
                            受験校にする
                          </button>
                          <button
                            title={
                              readOnly
                                ? "デモアカウントは閲覧専用です"
                                : undefined
                            }
                            onClick={() =>
                              runIfWritable(() =>
                                deleteMutation.mutate(goal.id)
                              )
                            }
                            className="ml-3 text-sm text-destructive hover:underline"
                          >
                            削除
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            「大学を探す」で気になる学部を候補に追加すると、ここで比較して受験校を決められます。
          </p>
        )}
      </section>
    </div>
  );
}
