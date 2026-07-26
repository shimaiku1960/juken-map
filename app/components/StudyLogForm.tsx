"use client";

import { useState } from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import {
  createStudyLogSchema,
  type CreateStudyLogInput,
} from "@/lib/validations/studyLog";
import {
  type StudyLog,
  useCreateStudyLog,
  useUpdateStudyLog,
} from "@/app/hooks/useStudyLogs";
import {
  useTextbooks,
  useTextbookMasters,
  useUpdateTextbookProgress,
} from "@/app/hooks/useTextbooks";
import { todayYmdTokyo, ymdLocal } from "@/lib/date";
import { RANGE_UNIT_VALUES } from "@/lib/validations/studyPlan";
import type { UpdateTextbookProgressInput } from "@/lib/validations/textbook";
import { subjectLabel } from "@/lib/subjects";
import {
  SubjectSelect,
  TextbookSelect,
  RangeUnitSelect,
} from "@/app/components/StudyFields";
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

// クイック加算ボタンの候補（分）
const QUICK_MINUTES = [15, 30, 60];

// 空のフォーム初期値（指定がなければ今日、時間は未入力）
const emptyValues = (initialDate = todayYmdTokyo()) => ({
  date: initialDate,
  subject: null,
  textbookId: null,
  rangeStart: null,
  rangeEnd: null,
  rangeUnit: null,
  memo: "",
});

export default function StudyLogForm({
  variant = "full",
  initialDate,
  initialLog,
  onSuccess,
}: {
  variant?: "full" | "quick";
  initialDate?: string;
  initialLog?: StudyLog;
  onSuccess?: () => void;
}) {
  const createLog = useCreateStudyLog();
  const updateLog = useUpdateStudyLog();
  const [detailsOpen, setDetailsOpen] = useState(initialLog != null);
  const { data: textbooks = [] } = useTextbooks();
  const { data: masters = [] } = useTextbookMasters();
  const updateProgress = useUpdateTextbookProgress();

  const form = useForm<CreateStudyLogInput>({
    resolver: zodResolver(createStudyLogSchema),
    defaultValues: initialLog
      ? {
          date: ymdLocal(initialLog.date),
          minutes: initialLog.minutes,
          subject: initialLog.subject,
          textbookId: initialLog.textbookId,
          rangeStart: initialLog.rangeStart,
          rangeEnd: initialLog.rangeEnd,
          rangeUnit: initialLog.rangeUnit,
          memo: initialLog.memo ?? "",
        }
      : emptyValues(initialDate),
  });
  const selectedTextbookId = useWatch({
    control: form.control,
    name: "textbookId",
  });
  const selectedTextbook = textbooks.find(
    (textbook) => textbook.id === selectedTextbookId
  );
  const isPlanLinked = initialLog?.studyPlanId != null;

  // 逆算設定済みの参考書を選んで単位を変えたら、その参考書の「追跡単位」を切り替える。
  // マスタ参考書ならその単位の総量も自動反映。1参考書=1単位で逆算の一貫性を保つ。
  const applyUnitToTextbook = (unit: string | null) => {
    if (
      initialLog ||
      unit == null ||
      !selectedTextbook ||
      selectedTextbook.totalAmount == null ||
      !RANGE_UNIT_VALUES.includes(unit) ||
      unit === selectedTextbook.rangeUnit
    ) {
      return;
    }
    const master = masters.find((m) => m.id === selectedTextbook.masterId);
    const metric = master?.metrics.find((mm) => mm.unit === unit);
    updateProgress.mutate({
      id: selectedTextbook.id,
      data: {
        totalAmount: metric?.totalAmount ?? selectedTextbook.totalAmount,
        rangeUnit: unit as NonNullable<UpdateTextbookProgressInput["rangeUnit"]>,
        targetDate: selectedTextbook.targetDate
          ? ymdLocal(selectedTextbook.targetDate)
          : null,
      },
    });
  };

  const onSubmit = (data: CreateStudyLogInput) => {
    const mutationOptions = {
      onSuccess: () => {
        toast.success(
          initialLog ? "学習実績を更新しました" : "学習実績を記録しました"
        );
        if (!initialLog) {
          // 日付は保ちつつ、他は空に戻して続けて記録できるようにする
          form.reset({ ...emptyValues(), date: form.getValues("date") });
        }
        onSuccess?.();
      },
      onError: (error: Error) => toast.error(error.message),
    };

    if (initialLog) {
      updateLog.mutate({ id: initialLog.id, data }, mutationOptions);
    } else {
      createLog.mutate(data, mutationOptions);
    }
  };
  const isPending = createLog.isPending || updateLog.isPending;

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className={
          variant === "quick"
            ? "flex min-h-0 flex-col gap-4"
            : "space-y-3"
        }
      >
        <div
          className={
            variant === "quick"
              ? "min-h-0 space-y-4 overflow-y-auto px-1"
              : "space-y-3"
          }
        >
          {/* 毎日使う学習時間を主役にし、補助項目は後から選べる順番にする。 */}
          <div className="flex flex-wrap items-end gap-3">
            {variant === "full" && (
              <FormField
                control={form.control}
                name="date"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>日付</FormLabel>
                    <FormControl>
                      <Input
                        type="date"
                        max={todayYmdTokyo()}
                        className="h-9 w-40"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            <FormField
              control={form.control}
              name="minutes"
              render={({ field }) => (
                <FormItem className={variant === "quick" ? "w-full" : undefined}>
                  <FormLabel>学習時間（分）</FormLabel>
                  <FormControl>
                    <div className="flex flex-wrap items-center gap-2">
                      <Input
                        type="number"
                        min={1}
                        placeholder="分"
                        autoFocus={variant === "quick"}
                        inputMode="numeric"
                        className={
                          variant === "quick"
                            ? "h-11 w-24 text-base [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                            : "h-9 w-20 [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                        }
                        value={field.value ?? ""}
                        onChange={(e) =>
                          field.onChange(
                            e.target.value === ""
                              ? undefined
                              : Number(e.target.value)
                          )
                        }
                      />
                      {QUICK_MINUTES.map((m) => (
                        <Button
                          key={m}
                          type="button"
                          variant="outline"
                          size="sm"
                          className={variant === "quick" ? "h-11" : undefined}
                          onClick={() =>
                            field.onChange(
                              variant === "quick" ? m : (field.value ?? 0) + m
                            )
                          }
                        >
                          {variant === "quick" ? `${m}分` : `+${m}`}
                        </Button>
                      ))}
                    </div>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="subject"
              render={({ field }) => (
                <FormItem className={variant === "quick" ? "w-full" : undefined}>
                  <FormLabel htmlFor={`study-log-subject-${variant}`}>
                    科目{variant === "quick" ? "（任意）" : ""}
                  </FormLabel>
                  <FormControl>
                    {isPlanLinked ? (
                      <Input
                        id={`study-log-subject-${variant}`}
                        value={subjectLabel(field.value)}
                        disabled
                        className={variant === "quick" ? "h-11" : undefined}
                      />
                    ) : (
                      <div>
                        <SubjectSelect
                          id={`study-log-subject-${variant}`}
                          ariaLabel="科目"
                          className={
                            variant === "quick" ? "h-11 w-full" : undefined
                          }
                          value={field.value}
                          onChange={field.onChange}
                        />
                      </div>
                    )}
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <details
            open={variant === "full" || detailsOpen}
            onToggle={(event) => {
              if (variant === "quick") {
                setDetailsOpen(event.currentTarget.open);
              }
            }}
            className={variant === "quick" ? "rounded-lg border p-3" : "contents"}
          >
            {variant === "quick" && (
              <summary className="cursor-pointer text-sm font-medium">
                {initialLog
                  ? "日付・教材・メモを編集"
                  : initialDate
                    ? "教材・メモを追加"
                    : "日付・教材・メモを追加"}
              </summary>
            )}
            <div
              className={
                variant === "quick" ? "mt-4 space-y-4" : "space-y-3"
              }
            >
              {variant === "quick" && (initialDate == null || initialLog) && (
                <FormField
                  control={form.control}
                  name="date"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>日付</FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          max={todayYmdTokyo()}
                          className="h-11 w-44"
                          disabled={isPlanLinked}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name="textbookId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>教材（任意）</FormLabel>
                    <FormControl>
                      <div>
                        {isPlanLinked ? (
                          <Input
                            id={`study-log-textbook-${variant}`}
                            value={
                              selectedTextbook?.name ??
                              initialLog?.textbook?.name ??
                              "参考書なし"
                            }
                            disabled
                            className={variant === "quick" ? "h-11" : undefined}
                          />
                        ) : (
                          <TextbookSelect
                            id={`study-log-textbook-${variant}`}
                            ariaLabel="教材"
                            className={
                              variant === "quick" ? "h-11 w-full" : undefined
                            }
                            value={field.value}
                            onChange={(textbookId) => {
                              field.onChange(textbookId);
                              if (textbookId == null) {
                                form.setValue("rangeStart", null, {
                                  shouldValidate: true,
                                });
                                form.setValue("rangeEnd", null, {
                                  shouldValidate: true,
                                });
                                form.setValue("rangeUnit", null, {
                                  shouldValidate: true,
                                });
                                return;
                              }
                              const textbook = textbooks.find(
                                (candidate) => candidate.id === textbookId
                              );
                              if (textbook?.rangeUnit != null) {
                                form.setValue("rangeUnit", textbook.rangeUnit, {
                                  shouldValidate: true,
                                });
                              }
                            }}
                          />
                        )}
                      </div>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {selectedTextbookId != null && (
                <div className="space-y-2">
                  <FormLabel>学習範囲（任意）</FormLabel>
                  <div className="flex flex-wrap items-start gap-2">
                    <FormField
                      control={form.control}
                      name="rangeStart"
                      render={({ field }) => (
                        <FormItem>
                          <FormControl>
                            <Input
                              aria-label="学習範囲の開始"
                              type="number"
                              min={1}
                              inputMode="numeric"
                              placeholder="開始"
                              className="h-11 w-24 [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                              value={field.value ?? ""}
                              onChange={(event) =>
                                field.onChange(
                                  event.target.value === ""
                                    ? null
                                    : Number(event.target.value)
                                )
                              }
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <span className="pt-3 text-sm text-muted-foreground">〜</span>
                    <FormField
                      control={form.control}
                      name="rangeEnd"
                      render={({ field }) => (
                        <FormItem>
                          <FormControl>
                            <Input
                              aria-label="学習範囲の終了"
                              type="number"
                              min={1}
                              inputMode="numeric"
                              placeholder="終了"
                              className="h-11 w-24 [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                              value={field.value ?? ""}
                              onChange={(event) =>
                                field.onChange(
                                  event.target.value === ""
                                    ? null
                                    : Number(event.target.value)
                                )
                              }
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="rangeUnit"
                      render={({ field }) => (
                        <FormItem>
                          <FormControl>
                            <RangeUnitSelect
                              ariaLabel="学習範囲の単位"
                              value={field.value}
                              onChange={(unit) => {
                                field.onChange(unit);
                                applyUnitToTextbook(unit);
                              }}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>
                </div>
              )}

              <FormField
                control={form.control}
                name="memo"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>メモ（任意）</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="学習内容や気づき"
                        className={variant === "quick" ? "h-11" : undefined}
                        value={field.value ?? ""}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </details>
        </div>

        <div
          className={
            variant === "quick"
              ? "-mx-4 -mb-4 shrink-0 border-t bg-popover px-4 pt-3 pb-4"
              : "flex justify-end"
          }
        >
          <Button
            type="submit"
            className={
              variant === "quick"
                ? "h-11 w-full sm:w-auto sm:min-w-28"
                : undefined
            }
            disabled={isPending}
          >
            {isPending
              ? initialLog
                ? "更新中…"
                : "記録中…"
              : initialLog
                ? "変更を保存"
                : "記録する"}
          </Button>
        </div>
      </form>
    </Form>
  );
}
