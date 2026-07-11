"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { goalSchema, type GoalInput } from "@/lib/validations/goal";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from "@/components/ui/form";
import { toast } from "sonner";
import { daysUntil, formatExamDate } from "@/lib/date";
import { useGoals, goalsKey, type Goal, type Faculty } from "@/app/hooks/useGoals";


type Props = {
  initialGoals: Goal[];
  faculties: Faculty[];
  isDemo: boolean;
};

export default function GoalList({ initialGoals, faculties, isDemo }: Props) {
  const queryClient = useQueryClient();

  // サーバー状態の取得。SSR で渡された initialGoals を初期キャッシュとして使う
  const { data: goals = [] } = useGoals(initialGoals);

  const form = useForm<GoalInput>({
    resolver: zodResolver(goalSchema),
    defaultValues: {
      facultyId: 0,
    },
  });

  // 追加
  const addMutation = useMutation({
    mutationFn: async (data: GoalInput) => {
      const res = await fetch("/api/goals", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "追加に失敗しました");
      }
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
      form.reset();
      toast.success("目標を追加しました");
    },
    onError: (error) => toast.error(error.message),
  });

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

  const onSubmit = (data: GoalInput) => addMutation.mutate(data);

  // 学部セレクトを大学ごとに optgroup でまとめる
  const facultiesByUniversity = faculties.reduce<Record<string, Faculty[]>>(
    (acc, faculty) => {
      const key = faculty.university.name;
      (acc[key] ??= []).push(faculty);
      return acc;
    },
    {}
  );

  const firstChoice = goals.find((goal) => goal.isFirstChoice);
  const others = goals.filter((goal) => !goal.isFirstChoice);

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
          <p className="text-sm text-gray-500">
            受験日 {formatExamDate(goal.faculty.examDate)}
            {days >= 0 && (
              <span className="ml-2 text-blue-600">あと{days}日</span>
            )}
          </p>
          <div className="flex flex-wrap gap-1">
            {goal.faculty.tags.map((tag) => (
              <span
                key={tag.name}
                className="text-xs bg-gray-100 text-gray-600 rounded px-2 py-0.5"
              >
                #{tag.name}
              </span>
            ))}
          </div>
        </div>
        {isDemo ? (
          goal.isFirstChoice && (
            <span className="text-sm shrink-0" title="第一志望">
              ★ 第一志望
            </span>
          )
        ) : (
          <div className="flex items-center gap-3 shrink-0">
            <button
              onClick={() =>
                firstChoiceMutation.mutate({
                  id: goal.id,
                  value: !goal.isFirstChoice,
                })
              }
              className="text-sm hover:underline"
              title={goal.isFirstChoice ? "第一志望を解除" : "第一志望にする"}
            >
              {goal.isFirstChoice ? "★ 第一志望" : "☆ 第一志望にする"}
            </button>
            <button
              onClick={() => deleteMutation.mutate(goal.id)}
              className="text-red-500 text-sm hover:underline"
            >
              削除
            </button>
          </div>
        )}
      </li>
    );
  };

  return (
    <div>
      {!isDemo && (
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className="flex gap-2 mb-6 items-start"
        >
          <FormField
            control={form.control}
            name="facultyId"
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormControl>
                  <select
                    value={field.value}
                    onChange={(e) => field.onChange(Number(e.target.value))}
                    className="border rounded px-3 py-2 w-full"
                  >
                    <option value={0}>志望学部を選択</option>
                    {Object.entries(facultiesByUniversity).map(
                      ([universityName, list]) => (
                        <optgroup key={universityName} label={universityName}>
                          {list.map((faculty) => (
                            <option key={faculty.id} value={faculty.id}>
                              {faculty.name}
                            </option>
                          ))}
                        </optgroup>
                      )
                    )}
                  </select>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <Button type="submit" disabled={addMutation.isPending}>
            追加
          </Button>
        </form>
      </Form>
      )}

      <section className="mb-6">
        <h3 className="text-sm font-bold text-gray-500 mb-2">第一志望</h3>
        {firstChoice ? (
          <ul>{renderGoalCard(firstChoice)}</ul>
        ) : (
          <p className="text-sm text-gray-400">
            まだ設定されていません（併願校から ☆ で設定できます）
          </p>
        )}
      </section>

      <section>
        <h3 className="text-sm font-bold text-gray-500 mb-2">併願校</h3>
        {others.length > 0 ? (
          <ul className="space-y-2">{others.map(renderGoalCard)}</ul>
        ) : (
          <p className="text-sm text-gray-400">併願校はありません</p>
        )}
      </section>
    </div>
  );
}
