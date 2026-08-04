"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { daysUntil, formatExamDate } from "@/lib/date";
import { useGoals, goalsKey, type Goal } from "@/app/hooks/useGoals";
import { notifyDemoReadOnly } from "@/lib/demo-client";
import { MoveHorizontal } from "lucide-react";
import { Badge } from "@/components/ui/badge";

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

  // 削除は元に戻せないうえ、スマホでは隣のボタンと近く誤タップしやすいので確認を挟む。
  // 学習予定・実績の削除（StudyDayPlanPanel / StudyHeatmap）と同じ window.confirm に揃える。
  const confirmAndDelete = (goal: Goal) =>
    runIfWritable(() => {
      const label = `${goal.faculty.university.name} ${goal.faculty.name}`;
      if (!window.confirm(`${label}を削除しますか？`)) return;
      deleteMutation.mutate(goal.id);
    });

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
        // スマホでは大学名と操作ボタンを同じ行に詰めると名前が途中で折り返すため、
        // 縦積みにして名前の幅を確保する。
        className="flex flex-col gap-2 rounded-lg border px-4 py-3 sm:flex-row sm:items-start sm:justify-between"
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
              <Badge key={tag.name} variant="secondary">
                #{tag.name}
              </Badge>
            ))}
          </div>
        </div>
        {/* 指で押す領域を 44px 確保する。左右の padding が隣のボタンとの間隔も兼ねる。 */}
        <div className="-mx-2 flex items-center sm:shrink-0">
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
            className="inline-flex min-h-11 items-center rounded-lg px-3 text-sm hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {goal.isFirstChoice ? "★ 第一志望" : "☆ 第一志望にする"}
          </button>
          <button
            title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
            onClick={() => confirmAndDelete(goal)}
            className="inline-flex min-h-11 items-center rounded-lg px-3 text-sm text-destructive hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
            {/* 表は 520px 必要なのでスマホでは全部映らない。横スクロールできることを明示する
                （右端のフェードだけでは気づけず、「あと◯日」が読めないままになる）。 */}
            <p className="mb-2 flex items-center gap-1 text-xs text-muted-foreground md:hidden">
              <MoveHorizontal aria-hidden="true" className="size-3.5" />
              表は横にスクロールできます
            </p>
            <div className="relative">
              {/* スクロールできる端であることを視覚的にも示す薄いフェード。 */}
              <div
                aria-hidden="true"
                className="pointer-events-none absolute inset-y-0 right-0 w-8 bg-gradient-to-l from-background to-transparent md:hidden"
              />
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
                                <Badge key={tag.name} variant="secondary">
                                  #{tag.name}
                                </Badge>
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
                              className="inline-flex min-h-11 items-center rounded-lg px-3 text-sm text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            >
                              受験校にする
                            </button>
                            <button
                              title={
                                readOnly
                                  ? "デモアカウントは閲覧専用です"
                                  : undefined
                              }
                              onClick={() => confirmAndDelete(goal)}
                              className="inline-flex min-h-11 items-center rounded-lg px-3 text-sm text-destructive hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
