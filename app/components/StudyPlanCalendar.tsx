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
import { useTextbooks, useCreateTextbook } from "@/app/hooks/useTextbooks";
import { SUBJECTS, subjectColor, subjectLabel } from "@/lib/subjects";
import { RANGE_UNITS } from "@/lib/validations/studyPlan";
import { studyPlanLabel } from "@/lib/studyPlan";
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

// 参考書セレクト（登録済みから選択＋新規追加）。空 = 参考書なし
const TextbookSelect = ({
  value,
  onChange,
}: {
  value: number | null | undefined;
  onChange: (v: number | null) => void;
}) => {
  const { data: textbooks = [] } = useTextbooks();
  const createTextbook = useCreateTextbook();
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");

  const submitNew = () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    createTextbook.mutate(trimmed, {
      onSuccess: (created) => {
        onChange(created.id); // 追加した参考書をそのまま選択状態に
        setName("");
        setAdding(false);
      },
      onError: (error) => toast.error(error.message),
    });
  };

  if (adding) {
    return (
      <div className="flex items-center gap-1">
        <Input
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="参考書名"
          className="h-9 w-40"
        />
        <Button
          type="button"
          size="sm"
          onClick={submitNew}
          disabled={createTextbook.isPending}
        >
          追加
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => {
            setAdding(false);
            setName("");
          }}
        >
          ✕
        </Button>
      </div>
    );
  }

  return (
    <select
      value={value ?? ""}
      onChange={(e) => {
        if (e.target.value === "__new__") {
          setAdding(true);
          return;
        }
        onChange(e.target.value ? Number(e.target.value) : null);
      }}
      className="h-9 rounded-md border bg-transparent px-2 text-sm"
    >
      <option value="">参考書なし</option>
      {textbooks.map((t) => (
        <option key={t.id} value={t.id}>
          {t.name}
        </option>
      ))}
      <option value="__new__">＋ 新しい参考書を追加</option>
    </select>
  );
};

// 範囲の単位セレクト（ページ/問題/章）。空 = 未選択
const RangeUnitSelect = ({
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
    <option value="">単位</option>
    {RANGE_UNITS.map((u) => (
      <option key={u.value} value={u.value}>
        {u.label}
      </option>
    ))}
  </select>
);

// 数値入力＋自前の増減ボタン。ユーザー好みで「上＝減る / 下＝増える」に固定する
// （ネイティブのスピナーは上＝増で反転できないため隠し、向きを自分で制御する）
const NumberStepper = ({
  value,
  onChange,
  placeholder,
  min = 1,
}: {
  value: number | null | undefined;
  onChange: (v: number | null) => void;
  placeholder?: string;
  min?: number;
}) => {
  const step = (delta: number) => {
    if (value == null) {
      // 空欄のとき：増やす(+1)は min から開始、減らす(-1)は空のまま
      onChange(delta > 0 ? min : null);
      return;
    }
    const next = value + delta;
    onChange(next < min ? min : next);
  };

  return (
    <div className="flex items-stretch gap-1">
      <Input
        type="number"
        min={min}
        placeholder={placeholder}
        className="h-9 w-16 text-center [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
        value={value ?? ""}
        onChange={(e) =>
          onChange(e.target.value === "" ? null : Number(e.target.value))
        }
      />
      <div className="flex flex-col">
        {/* 上ボタン＝減らす */}
        <button
          type="button"
          aria-label="1つ減らす"
          onClick={() => step(-1)}
          className="flex h-[18px] w-6 items-center justify-center rounded-t-md border text-[10px] leading-none hover:bg-accent"
        >
          ▲
        </button>
        {/* 下ボタン＝増やす */}
        <button
          type="button"
          aria-label="1つ増やす"
          onClick={() => step(1)}
          className="flex h-[18px] w-6 items-center justify-center rounded-b-md border border-t-0 text-[10px] leading-none hover:bg-accent"
        >
          ▼
        </button>
      </div>
    </div>
  );
};

const todayStr = () => toDateStr(new Date());

// 追加フォームの空の1件（参考書・範囲・メモ・科目すべて未入力）
const emptyItem = () => ({
  textbookId: null,
  rangeStart: null,
  rangeEnd: null,
  rangeUnit: null,
  content: "",
  subject: null,
});

export default function StudyPlanCalendar({
  initialPlans,
  initialGoals = [],
  isDemo,
}: {
  initialPlans: StudyPlan[];
  initialGoals?: Goal[];
  isDemo: boolean;
}) {
  const queryClient = useQueryClient();
  const { data: plans = [] } = useStudyPlans(initialPlans);
  const { data: goals = [] } = useGoals(initialGoals);

  // 「その日のダイアログ」（確認＋追加＋編集導線を1か所に集約）
  const [selectedDate, setSelectedDate] = useState<string | null>(null);
  // 1件だけを編集するダイアログ
  const [editingPlan, setEditingPlan] = useState<StudyPlan | null>(null);
  // 受験イベントクリックで開く志望校ダイアログ（ユーザー資産の編集）
  const [examGoalId, setExamGoalId] = useState<number | null>(null);
  const [noteDraft, setNoteDraft] = useState("");
  const examGoal = goals.find((g) => g.id === examGoalId) ?? null;

  // 選択中の日の予定（最新の plans から都度絞り込む）
  const dayPlans = selectedDate
    ? plans.filter((p) => toDateStr(p.date) === selectedDate)
    : [];

  // 追加フォーム：選択中の日付に複数の内容（items）を登録
  const createForm = useForm<CreateStudyPlansInput>({
    resolver: zodResolver(createStudyPlansSchema),
    defaultValues: {
      date: todayStr(),
      items: [emptyItem()],
    },
  });
  const { fields, append, remove } = useFieldArray({
    control: createForm.control,
    name: "items",
  });

  const editForm = useForm<UpdateStudyPlanInput>({
    resolver: zodResolver(updateStudyPlanSchema),
    defaultValues: {
      date: "",
      content: "",
      subject: null,
      textbookId: null,
      rangeStart: null,
      rangeEnd: null,
      rangeUnit: null,
    },
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
      // ダイアログは開いたまま。追加欄だけ空に戻して続けて足せるようにする
      createForm.reset({
        date: createForm.getValues("date"),
        items: [{ content: "", subject: null }],
      });
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
      setEditingPlan(null);
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
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "更新に失敗しました");
      }
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
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "削除に失敗しました");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: studyPlansKey });
      toast.success("削除しました");
      setEditingPlan(null);
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
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "削除に失敗しました");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: goalsKey });
      toast.success("志望校から削除しました");
      setExamGoalId(null);
    },
    onError: (error) => toast.error(error.message),
  });

  // 日をクリック → その日のダイアログを開く（追加フォームの日付もその日にセット）
  const openDay = (date: string) => {
    createForm.reset({ date, items: [{ content: "", subject: null }] });
    setSelectedDate(date);
  };

  // 一覧の「編集」→ 1件編集ダイアログへ（日ダイアログは閉じてネストを避ける）
  const openEdit = (plan: StudyPlan) => {
    editForm.reset({
      date: toDateStr(plan.date),
      content: plan.content ?? undefined,
      subject: plan.subject,
      textbookId: plan.textbookId,
      rangeStart: plan.rangeStart,
      rangeEnd: plan.rangeEnd,
      rangeUnit: plan.rangeUnit,
    });
    setSelectedDate(null);
    setEditingPlan(plan);
  };

  // 受験イベントクリック → 志望校ダイアログを開く（メモ下書きを現在値で初期化）
  const openExam = (goal: Goal) => {
    setNoteDraft(goal.note ?? "");
    setExamGoalId(goal.id);
  };

  // FullCalendar 用イベント（学習予定）。科目で色分け、完了は打ち消し線
  const events = plans.map((p) => ({
    id: String(p.id),
    title: studyPlanLabel(p),
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
        {isDemo ? (
          <>
            デモアカウントは閲覧専用です。予定の閲覧のみ可能です（登録・編集・削除はできません）。
          </>
        ) : (
          <>
            日をクリックすると、その日の予定の確認・追加・編集ができます。
            予定をドラッグすると別の日へ移動できます。
            <span className="text-red-600">赤色</span>
            は志望校の受験日です（クリックで第一志望・メモの編集や削除ができます）。
          </>
        )}
      </p>

      {/* FullCalendar 本体 */}
      <StudyFullCalendar
        events={events}
        examEvents={examEvents}
        editable={!isDemo}
        onEventClick={(id) => {
          if (id.startsWith("exam-")) {
            const goalId = Number(id.slice("exam-".length));
            const goal = goals.find((g) => g.id === goalId);
            if (goal) openExam(goal);
            return;
          }
          const plan = plans.find((p) => String(p.id) === id);
          if (plan) openDay(toDateStr(plan.date));
        }}
        onDateClick={(date) => openDay(date)}
        onEventDrop={(id, newDate) =>
          updateMutation.mutate({ id: Number(id), data: { date: newDate } })
        }
      />

      {/* 日付順リスト（チェックで完了、詳細でその日のダイアログ） */}
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
                  disabled={isDemo}
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
                    {studyPlanLabel(plan)}
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => openDay(toDateStr(plan.date))}
                >
                  詳細
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* その日のダイアログ：既存予定の確認・完了・削除・編集 ＋ 追加を1か所に */}
      <Dialog
        open={selectedDate !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedDate(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{selectedDate}</DialogTitle>
          </DialogHeader>

          {/* その日の予定一覧 */}
          {dayPlans.length === 0 ? (
            <p className="text-sm text-gray-500">この日の予定はまだありません。</p>
          ) : (
            <ul className="space-y-2">
              {dayPlans.map((plan) => (
                <li
                  key={plan.id}
                  className="flex items-center gap-3 border rounded px-3 py-2"
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
                  <span
                    className={`flex-1 text-sm ${
                      plan.done ? "line-through text-gray-400" : ""
                    }`}
                  >
                    {studyPlanLabel(plan)}
                  </span>
                  {!isDemo && (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => openEdit(plan)}
                      >
                        編集
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => deleteMutation.mutate(plan.id)}
                        disabled={deleteMutation.isPending}
                      >
                        削除
                      </Button>
                    </>
                  )}
                </li>
              ))}
            </ul>
          )}

          {/* 追加エリア（その日で複数登録）。デモは閲覧専用のため非表示 */}
          {!isDemo && (
          <div className="border-t pt-4">
            <Form {...createForm}>
              <form
                onSubmit={createForm.handleSubmit((data) =>
                  createMutation.mutate(data)
                )}
                className="space-y-3"
              >
                <p className="text-sm font-medium leading-none">予定を追加</p>
                {fields.map((f, index) => (
                  <div
                    key={f.id}
                    className="space-y-2 rounded-md border p-3"
                  >
                    {/* 上段：科目・参考書と、この1件の削除 */}
                    <div className="flex items-center gap-2">
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
                        name={`items.${index}.textbookId`}
                        render={({ field }) => (
                          <TextbookSelect
                            value={field.value}
                            onChange={field.onChange}
                          />
                        )}
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="ml-auto"
                        onClick={() => remove(index)}
                        disabled={fields.length === 1}
                        aria-label="この予定を削除"
                      >
                        ✕
                      </Button>
                    </div>

                    {/* 中段：範囲（開始〜終了）＋単位 */}
                    <div className="flex items-center gap-2">
                      <FormField
                        control={createForm.control}
                        name={`items.${index}.rangeStart`}
                        render={({ field }) => (
                          <FormItem>
                            <FormControl>
                              <NumberStepper
                                placeholder="開始"
                                value={field.value}
                                onChange={field.onChange}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <span className="text-sm text-gray-500">〜</span>
                      <FormField
                        control={createForm.control}
                        name={`items.${index}.rangeEnd`}
                        render={({ field }) => (
                          <FormItem>
                            <FormControl>
                              <NumberStepper
                                placeholder="終了"
                                value={field.value}
                                onChange={field.onChange}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={createForm.control}
                        name={`items.${index}.rangeUnit`}
                        render={({ field }) => (
                          <FormItem>
                            <FormControl>
                              <RangeUnitSelect
                                value={field.value}
                                onChange={field.onChange}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>

                    {/* 下段：任意メモ */}
                    <FormField
                      control={createForm.control}
                      name={`items.${index}.content`}
                      render={({ field }) => (
                        <FormItem>
                          <FormControl>
                            <Input
                              placeholder="メモ（任意）"
                              value={field.value ?? ""}
                              onChange={field.onChange}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>
                ))}
                <div className="flex items-center justify-between">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => append(emptyItem())}
                  >
                    ＋ 内容を追加
                  </Button>
                  <Button type="submit" disabled={createMutation.isPending}>
                    登録
                  </Button>
                </div>
              </form>
            </Form>
          </div>
          )}
        </DialogContent>
      </Dialog>

      {/* 1件編集ダイアログ */}
      <Dialog
        open={editingPlan !== null}
        onOpenChange={(open) => {
          if (!open) setEditingPlan(null);
        }}
      >
        <DialogContent>
          {editingPlan && (
            <>
              <DialogHeader>
                <DialogTitle>学習予定を編集</DialogTitle>
              </DialogHeader>
              <Form {...editForm}>
                <form
                  onSubmit={editForm.handleSubmit((data) =>
                    updateMutation.mutate({ id: editingPlan.id, data })
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
                    name="textbookId"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>参考書</FormLabel>
                        <FormControl>
                          <div>
                            <TextbookSelect
                              value={field.value}
                              onChange={field.onChange}
                            />
                          </div>
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <div className="space-y-2">
                    <p className="text-sm font-medium">範囲</p>
                    <div className="flex items-center gap-2">
                      <FormField
                        control={editForm.control}
                        name="rangeStart"
                        render={({ field }) => (
                          <FormItem>
                            <FormControl>
                              <NumberStepper
                                placeholder="開始"
                                value={field.value}
                                onChange={field.onChange}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <span className="text-sm text-gray-500">〜</span>
                      <FormField
                        control={editForm.control}
                        name="rangeEnd"
                        render={({ field }) => (
                          <FormItem>
                            <FormControl>
                              <NumberStepper
                                placeholder="終了"
                                value={field.value}
                                onChange={field.onChange}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={editForm.control}
                        name="rangeUnit"
                        render={({ field }) => (
                          <FormItem>
                            <FormControl>
                              <RangeUnitSelect
                                value={field.value}
                                onChange={field.onChange}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>
                  <FormField
                    control={editForm.control}
                    name="content"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>メモ（任意）</FormLabel>
                        <FormControl>
                          <Input
                            value={field.value ?? ""}
                            onChange={field.onChange}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <DialogFooter className="gap-2 sm:gap-0">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => deleteMutation.mutate(editingPlan.id)}
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

              {/* 第一志望トグル（ユーザー資産）。デモは静的ラベル表示のみ */}
              {isDemo ? (
                examGoal.isFirstChoice && (
                  <p className="text-sm font-medium">★ 第一志望</p>
                )
              ) : (
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
              )}

              {/* メモ（ユーザー資産）。デモは読み取り表示のみ */}
              {isDemo ? (
                examGoal.note && (
                  <div className="space-y-1">
                    <label className="text-sm font-medium">メモ・備考</label>
                    <p className="whitespace-pre-wrap rounded-md border px-3 py-2 text-sm">
                      {examGoal.note}
                    </p>
                  </div>
                )
              ) : (
                <div className="space-y-1">
                  <label className="text-sm font-medium">メモ・備考</label>
                  <textarea
                    className="w-full min-h-24 rounded-md border px-3 py-2 text-sm"
                    placeholder="会場・持ち物・対策メモなど"
                    value={noteDraft}
                    onChange={(e) => setNoteDraft(e.target.value)}
                  />
                </div>
              )}

              {!isDemo && (
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
                        data: {
                          note: noteDraft.trim() === "" ? null : noteDraft,
                        },
                      })
                    }
                    disabled={goalPatchMutation.isPending}
                  >
                    メモを保存
                  </Button>
                </DialogFooter>
              )}
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
