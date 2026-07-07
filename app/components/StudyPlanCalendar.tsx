"use client";

import { useState } from "react";
import { useForm, useFieldArray } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  createStudyPlansSchema,
  updateStudyPlanSchema,
  type CreateStudyPlansInput,
  type UpdateStudyPlanInput,
} from "@/lib/validations/studyPlan";
import {
  useStudyPlans,
  studyPlansKey,
  type StudyPlan,
} from "@/app/hooks/useStudyPlans";
import { useGoals, goalsKey, type Goal } from "@/app/hooks/useGoals";
import { SUBJECTS, subjectColor, subjectLabel } from "@/lib/subjects";
import StudyFullCalendar from "@/app/components/StudyFullCalendar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";

// 文字列 or Date → "YYYY-MM-DD"（表示・フォーム初期値・カレンダー用に統一）
const toDateStr = (d: string | Date) =>
  typeof d === "string" ? d.slice(0, 10) : d.toISOString().slice(0, 10);

// 科目セレクト（固定リスト。空 = 科目なし）
const SubjectSelect = ({
  value,
  onChange,
}: {
  value: string | null | undefined;
  onChange: (v: string | null) => void;
}) => (
  <select
    value={value ?? ""}
    onChange={(e) => onChange(e.target.value || null)}
    className="h-9 rounded-md border bg-transparent px-2 text-sm"
  >
    <option value="">科目なし</option>
    {SUBJECTS.map((s) => (
      <option key={s.value} value={s.value}>
        {s.label}
      </option>
    ))}
  </select>
);

// 選択中の予定をどのモードで開いているか（Google風: まず閲覧→編集）
type Selection = { plan: StudyPlan; mode: "view" | "edit" } | null;

const todayStr = () => toDateStr(new Date());

export default function StudyPlanCalendar({
  initialPlans,
  initialGoals = [],
}: {
  initialPlans: StudyPlan[];
  initialGoals?: Goal[];
}) {
  const queryClient = useQueryClient();
  const { data: plans = [] } = useStudyPlans(initialPlans);
  const { data: goals = [] } = useGoals(initialGoals);

  const [selection, setSelection] = useState<Selection>(null);
  // 日付クリックで開く「その日で登録」ダイアログ
  const [createOpen, setCreateOpen] = useState(false);
  // 受験イベントクリックで開く志望校ダイアログ（ユーザー資産の編集）
  const [examGoalId, setExamGoalId] = useState<number | null>(null);
  const [noteDraft, setNoteDraft] = useState("");
  const examGoal = goals.find((g) => g.id === examGoalId) ?? null;

  // モーダルは開いた瞬間のコピーでなく、常に最新の plans から引き直す
  const selectedPlan = selection
    ? plans.find((p) => p.id === selection.plan.id) ?? selection.plan
    : null;

  // 作成フォーム：1つの日付に複数の内容（items）を登録
  const createForm = useForm<CreateStudyPlansInput>({
    resolver: zodResolver(createStudyPlansSchema),
    defaultValues: {
      date: todayStr(),
      items: [{ content: "", subject: null }],
    },
  });
  const { fields, append, remove } = useFieldArray({
    control: createForm.control,
    name: "items",
  });

  const editForm = useForm<UpdateStudyPlanInput>({
    resolver: zodResolver(updateStudyPlanSchema),
    defaultValues: { date: "", content: "", subject: null },
  });

  const createMutation = useMutation({
    mutationFn: async (data: CreateStudyPlansInput) => {
      const res = await fetch("/api/study-plans", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "登録に失敗しました");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: studyPlansKey });
      toast.success("学習予定を登録しました");
      setCreateOpen(false);
    },
    onError: (error) => toast.error(error.message),
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      data,
    }: {
      id: number;
      data: UpdateStudyPlanInput;
    }) => {
      const res = await fetch(`/api/study-plans/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "更新に失敗しました");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: studyPlansKey });
      toast.success("更新しました");
      setSelection(null);
    },
    onError: (error) => toast.error(error.message),
  });

  // 完了トグルは楽観的更新（押した瞬間に反映→失敗時のみ巻き戻し）
  const toggleDoneMutation = useMutation({
    mutationFn: async ({ id, done }: { id: number; done: boolean }) => {
      const res = await fetch(`/api/study-plans/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ done }),
      });
      if (!res.ok) throw new Error("更新に失敗しました");
    },
    onMutate: async ({ id, done }) => {
      await queryClient.cancelQueries({ queryKey: studyPlansKey });
      const prev = queryClient.getQueryData<StudyPlan[]>(studyPlansKey);
      queryClient.setQueryData<StudyPlan[]>(studyPlansKey, (old) =>
        (old ?? []).map((p) => (p.id === id ? { ...p, done } : p))
      );
      return { prev };
    },
    onError: (error, _vars, context) => {
      if (context?.prev) {
        queryClient.setQueryData(studyPlansKey, context.prev);
      }
      toast.error(error.message);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: studyPlansKey });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      const res = await fetch(`/api/study-plans/${id}`, { method: "DELETE" });
      if (!res.ok) throw new Error("削除に失敗しました");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: studyPlansKey });
      toast.success("削除しました");
      setSelection(null);
    },
    onError: (error) => toast.error(error.message),
  });

  // 志望校（受験イベント）のユーザー資産更新：第一志望トグル / メモ
  const goalPatchMutation = useMutation({
    mutationFn: async ({
      id,
      data,
    }: {
      id: number;
      data: { isFirstChoice?: boolean; note?: string | null };
    }) => {
      const res = await fetch(`/api/goals/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "更新に失敗しました");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
      toast.success("更新しました");
    },
    onError: (error) => toast.error(error.message),
  });

  const goalDeleteMutation = useMutation({
    mutationFn: async (id: number) => {
      const res = await fetch(`/api/goals/${id}`, { method: "DELETE" });
      if (!res.ok) throw new Error("削除に失敗しました");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
      toast.success("志望校から削除しました");
      setExamGoalId(null);
    },
    onError: (error) => toast.error(error.message),
  });

  // 予定クリック → まず閲覧（クイックビュー）
  const openView = (plan: StudyPlan) => setSelection({ plan, mode: "view" });

  // 受験イベントクリック → 志望校ダイアログを開く（メモ下書きを現在値で初期化）
  const openExam = (goal: Goal) => {
    setNoteDraft(goal.note ?? "");
    setExamGoalId(goal.id);
  };

  // 閲覧 → 編集へ切り替え（フォームに現在値をセット）
  const switchToEdit = () => {
    if (!selection) return;
    editForm.reset({
      date: toDateStr(selection.plan.date),
      content: selection.plan.content,
      subject: selection.plan.subject,
    });
    setSelection({ plan: selection.plan, mode: "edit" });
  };

  // FullCalendar 用イベント（学習予定）。科目で色分け、完了は打ち消し線
  const events = plans.map((p) => ({
    id: String(p.id),
    title: p.content,
    date: toDateStr(p.date),
    color: subjectColor(p.subject),
    done: p.done,
  }));

  // 受験日程イベント（読み取り専用。第一志望は ★ を付ける）
  const examEvents = goals.map((g) => ({
    id: `exam-${g.id}`,
    title: `【受験】${g.faculty.university.name} ${g.faculty.name}${
      g.isFirstChoice ? " ★" : ""
    }`,
    date: toDateStr(g.faculty.examDate),
  }));

  return (
    <div className="space-y-8">
      <p className="text-sm text-gray-500">
        空いている日をクリックするとその日で登録、予定をクリックすると詳細表示、
        予定をドラッグすると別の日へ移動できます。
        <span className="text-red-600">赤色</span>
        は志望校の受験日です（クリックで第一志望・メモの編集や削除ができます）。
      </p>

      {/* FullCalendar 本体 */}
      <StudyFullCalendar
        events={events}
        examEvents={examEvents}
        onEventClick={(id) => {
          if (id.startsWith("exam-")) {
            const goalId = Number(id.slice("exam-".length));
            const goal = goals.find((g) => g.id === goalId);
            if (goal) openExam(goal);
            return;
          }
          const plan = plans.find((p) => String(p.id) === id);
          if (plan) openView(plan);
        }}
        onDateClick={(date) => {
          createForm.reset({ date, items: [{ content: "", subject: null }] });
          setCreateOpen(true);
        }}
        onEventDrop={(id, newDate) =>
          updateMutation.mutate({ id: Number(id), data: { date: newDate } })
        }
      />

      {/* 日付順リスト（チェックで完了、クリックで詳細） */}
      <div>
        <h2 className="text-xl font-bold mb-3">予定一覧</h2>
        {plans.length === 0 ? (
          <p className="text-sm text-gray-500">
            まだ予定がありません。カレンダーの日付をクリックして登録しましょう。
          </p>
        ) : (
          <ul className="space-y-2">
            {plans.map((plan) => (
              <li
                key={plan.id}
                className="border rounded px-4 py-3 flex items-center gap-3"
              >
                <input
                  type="checkbox"
                  className="h-4 w-4 shrink-0"
                  checked={plan.done}
                  onChange={() =>
                    toggleDoneMutation.mutate({
                      id: plan.id,
                      done: !plan.done,
                    })
                  }
                  aria-label="完了"
                />
                <span
                  className="h-3 w-3 shrink-0 rounded-full"
                  style={{ backgroundColor: subjectColor(plan.subject) }}
                  title={subjectLabel(plan.subject)}
                />
                <div className="flex-1">
                  <p className="text-sm text-gray-500">{toDateStr(plan.date)}</p>
                  <p
                    className={`font-medium ${
                      plan.done ? "line-through text-gray-400" : ""
                    }`}
                  >
                    {plan.content}
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => openView(plan)}
                >
                  詳細
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* 日付クリック → その日で複数登録するダイアログ */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>学習予定を登録</DialogTitle>
          </DialogHeader>
          <Form {...createForm}>
            <form
              onSubmit={createForm.handleSubmit((data) =>
                createMutation.mutate(data)
              )}
              className="space-y-4"
            >
              <FormField
                control={createForm.control}
                name="date"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>日付</FormLabel>
                    <FormControl>
                      <Input type="date" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* 内容（複数） */}
              <div className="space-y-2">
                <p className="text-sm font-medium leading-none">内容</p>
                {fields.map((f, index) => (
                  <div key={f.id} className="flex items-start gap-2">
                    <FormField
                      control={createForm.control}
                      name={`items.${index}.subject`}
                      render={({ field }) => (
                        <SubjectSelect
                          value={field.value}
                          onChange={field.onChange}
                        />
                      )}
                    />
                    <FormField
                      control={createForm.control}
                      name={`items.${index}.content`}
                      render={({ field }) => (
                        <FormItem className="flex-1">
                          <FormControl>
                            <Input
                              autoFocus={index === 0}
                              placeholder="英単語 300〜400 / 数学 青チャ p.20"
                              {...field}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => remove(index)}
                      disabled={fields.length === 1}
                      aria-label="この内容を削除"
                    >
                      ✕
                    </Button>
                  </div>
                ))}
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => append({ content: "", subject: null })}
                >
                  ＋ 内容を追加
                </Button>
              </div>

              <DialogFooter>
                <Button type="submit" disabled={createMutation.isPending}>
                  登録
                </Button>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>

      {/* クイックビュー / 編集モーダル（Google風の2段階） */}
      <Dialog
        open={selection !== null}
        onOpenChange={(open) => {
          if (!open) setSelection(null);
        }}
      >
        <DialogContent>
          {selectedPlan && selection?.mode === "view" && (
            <>
              <DialogHeader>
                <DialogTitle>学習予定</DialogTitle>
              </DialogHeader>
              <div className="space-y-1">
                <p className="text-sm text-gray-500">
                  {toDateStr(selectedPlan.date)} ・{" "}
                  {subjectLabel(selectedPlan.subject)}
                </p>
                <p
                  className={`text-lg font-medium ${
                    selectedPlan.done ? "line-through text-gray-400" : ""
                  }`}
                >
                  {selectedPlan.content}
                </p>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  className="h-4 w-4"
                  checked={selectedPlan.done}
                  onChange={() =>
                    toggleDoneMutation.mutate({
                      id: selectedPlan.id,
                      done: !selectedPlan.done,
                    })
                  }
                />
                完了にする
              </label>
              <DialogFooter className="gap-2 sm:gap-0">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => deleteMutation.mutate(selectedPlan.id)}
                  disabled={deleteMutation.isPending}
                >
                  削除
                </Button>
                <Button type="button" onClick={switchToEdit}>
                  編集
                </Button>
              </DialogFooter>
            </>
          )}

          {selection?.mode === "edit" && (
            <>
              <DialogHeader>
                <DialogTitle>学習予定を編集</DialogTitle>
              </DialogHeader>
              <Form {...editForm}>
                <form
                  onSubmit={editForm.handleSubmit((data) =>
                    updateMutation.mutate({ id: selection.plan.id, data })
                  )}
                  className="space-y-4"
                >
                  <FormField
                    control={editForm.control}
                    name="date"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>日付</FormLabel>
                        <FormControl>
                          <Input type="date" {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={editForm.control}
                    name="subject"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>科目</FormLabel>
                        <FormControl>
                          <div>
                            <SubjectSelect
                              value={field.value}
                              onChange={field.onChange}
                            />
                          </div>
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={editForm.control}
                    name="content"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>内容</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <DialogFooter className="gap-2 sm:gap-0">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => deleteMutation.mutate(selection.plan.id)}
                      disabled={deleteMutation.isPending}
                    >
                      削除
                    </Button>
                    <Button type="submit" disabled={updateMutation.isPending}>
                      保存
                    </Button>
                  </DialogFooter>
                </form>
              </Form>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* 受験日程（志望校）ダイアログ。受験日など master は読み取り専用、
          ユーザー資産（第一志望・メモ・削除）だけ編集可 */}
      <Dialog
        open={examGoal !== null}
        onOpenChange={(open) => {
          if (!open) setExamGoalId(null);
        }}
      >
        <DialogContent>
          {examGoal && (
            <>
              <DialogHeader>
                <DialogTitle>
                  {examGoal.faculty.university.name} {examGoal.faculty.name}
                </DialogTitle>
              </DialogHeader>

              {/* 読み取り専用（マスター） */}
              <p className="text-sm text-gray-500">
                受験日：{toDateStr(examGoal.faculty.examDate)}
              </p>

              {/* 第一志望トグル（ユーザー資産） */}
              <Button
                type="button"
                variant={examGoal.isFirstChoice ? "default" : "outline"}
                onClick={() =>
                  goalPatchMutation.mutate({
                    id: examGoal.id,
                    data: { isFirstChoice: !examGoal.isFirstChoice },
                  })
                }
                disabled={goalPatchMutation.isPending}
              >
                {examGoal.isFirstChoice ? "★ 第一志望" : "☆ 第一志望にする"}
              </Button>

              {/* メモ（ユーザー資産） */}
              <div className="space-y-1">
                <label className="text-sm font-medium">メモ・備考</label>
                <textarea
                  className="w-full min-h-24 rounded-md border px-3 py-2 text-sm"
                  placeholder="会場・持ち物・対策メモなど"
                  value={noteDraft}
                  onChange={(e) => setNoteDraft(e.target.value)}
                />
              </div>

              <DialogFooter className="gap-2 sm:gap-0">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => goalDeleteMutation.mutate(examGoal.id)}
                  disabled={goalDeleteMutation.isPending}
                >
                  志望校から削除
                </Button>
                <Button
                  type="button"
                  onClick={() =>
                    goalPatchMutation.mutate({
                      id: examGoal.id,
                      data: { note: noteDraft.trim() === "" ? null : noteDraft },
                    })
                  }
                  disabled={goalPatchMutation.isPending}
                >
                  メモを保存
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
