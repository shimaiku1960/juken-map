"use client";

import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  completeStudyPlanSchema,
  type CompleteStudyPlanInput,
} from "@/lib/validations/studyLog";
import { studyLogsKey } from "@/app/hooks/useStudyLogs";
import { studyPlansKey } from "@/app/hooks/useStudyPlans";
import { subjectLabel } from "@/lib/subjects";
import { RangeUnitSelect, NumberStepper } from "@/app/components/StudyFields";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import type { TodayPlan } from "@/app/components/TodayStudyPlans";

const QUICK_MINUTES = [15, 30, 60];

const valuesFor = (plan: TodayPlan | null): CompleteStudyPlanInput => ({
  minutes: undefined as unknown as number,
  rangeStart: plan?.rangeStart ?? null,
  rangeEnd: plan?.rangeEnd ?? null,
  rangeUnit: plan?.rangeUnit ?? null,
  memo: "",
});

export default function QuickStudyLogDialog({
  plan,
  open,
  onOpenChange,
  onSuccess,
}: {
  plan: TodayPlan | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: (planId: number, minutes: number) => void;
}) {
  const queryClient = useQueryClient();
  const form = useForm<CompleteStudyPlanInput>({
    resolver: zodResolver(completeStudyPlanSchema),
    defaultValues: valuesFor(plan),
  });

  useEffect(() => {
    if (open) form.reset(valuesFor(plan));
  }, [form, open, plan]);

  const complete = useMutation({
    mutationFn: async (data: CompleteStudyPlanInput) => {
      if (!plan) throw new Error("予定が見つかりません");
      const response = await fetch(`/api/study-plans/${plan.id}/complete`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!response.ok) {
        const body = await response.json();
        throw new Error(
          typeof body.error === "string" ? body.error : "実績を記録できませんでした"
        );
      }
      return response.json();
    },
    onSuccess: (_result, variables) => {
      if (!plan) return;
      onSuccess(plan.id, variables.minutes);
      queryClient.invalidateQueries({ queryKey: studyLogsKey });
      queryClient.invalidateQueries({ queryKey: studyPlansKey });
      toast.success("実績を記録し、予定を完了しました");
      onOpenChange(false);
    },
  });

  const onSubmit = (data: CompleteStudyPlanInput) => complete.mutate(data);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!complete.isPending) onOpenChange(nextOpen);
      }}
    >
      <DialogContent
        showCloseButton={!complete.isPending}
        className="top-auto bottom-0 max-h-[90dvh] translate-y-0 overflow-y-auto rounded-b-none sm:top-1/2 sm:bottom-auto sm:max-w-lg sm:-translate-y-1/2 sm:rounded-xl"
      >
        <DialogHeader>
          <DialogTitle>実績を記録</DialogTitle>
          <DialogDescription>
            予定の内容を引き継いで、実際に学習した時間を記録します。
          </DialogDescription>
        </DialogHeader>

        {plan && (
          <div className="rounded-lg bg-muted p-3">
            <p className="font-medium">{plan.content}</p>
            <p className="mt-1 text-sm text-muted-foreground">
              {subjectLabel(plan.subject)}
              {plan.textbookName && ` ・ ${plan.textbookName}`}
            </p>
          </div>
        )}

        <Form {...form}>
          <form
            id="quick-study-log-form"
            onSubmit={form.handleSubmit(onSubmit)}
            className="min-w-0 space-y-4"
          >
            {complete.error && (
              <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                {complete.error.message}。入力内容を保ったまま、もう一度お試しください。
              </p>
            )}

            <FormField
              control={form.control}
              name="minutes"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>学習時間（必須）</FormLabel>
                  <FormControl>
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <Input
                          autoFocus
                          type="number"
                          min={1}
                          max={1440}
                          inputMode="numeric"
                          className="h-11 w-28 text-base"
                          value={field.value ?? ""}
                          onChange={(event) =>
                            field.onChange(
                              event.target.value === ""
                                ? undefined
                                : Number(event.target.value)
                            )
                          }
                        />
                        <span>分</span>
                      </div>
                      <div className="grid grid-cols-3 gap-2">
                        {QUICK_MINUTES.map((minutes) => (
                          <Button
                            key={minutes}
                            type="button"
                            variant="outline"
                            className="h-11"
                            onClick={() => field.onChange(minutes)}
                          >
                            {minutes}分
                          </Button>
                        ))}
                      </div>
                    </div>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div>
              <p className="mb-2 text-sm font-medium">実施範囲（必要なら修正）</p>
              <div className="flex min-w-0 flex-wrap items-start gap-2">
                <FormField
                  control={form.control}
                  name="rangeStart"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <NumberStepper
                          ariaLabel="実施範囲の開始"
                          placeholder="開始"
                          value={field.value}
                          onChange={field.onChange}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <span className="pt-2 text-muted-foreground">〜</span>
                <FormField
                  control={form.control}
                  name="rangeEnd"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <NumberStepper
                          ariaLabel="実施範囲の終了"
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
                  control={form.control}
                  name="rangeUnit"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <RangeUnitSelect
                          ariaLabel="範囲の単位"
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
              control={form.control}
              name="memo"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>メモ（任意）</FormLabel>
                  <FormControl>
                    <Input value={field.value ?? ""} onChange={field.onChange} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>

        <DialogFooter className="sticky bottom-0">
          <Button
            type="button"
            variant="outline"
            disabled={complete.isPending}
            onClick={() => onOpenChange(false)}
          >
            キャンセル
          </Button>
          <Button
            type="submit"
            form="quick-study-log-form"
            disabled={complete.isPending}
          >
            {complete.isPending ? "記録中…" : "記録して完了"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
