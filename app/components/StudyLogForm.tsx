"use client";

import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import {
  createStudyLogSchema,
  type CreateStudyLogInput,
} from "@/lib/validations/studyLog";
import { useCreateStudyLog } from "@/app/hooks/useStudyLogs";
import { useTextbooks } from "@/app/hooks/useTextbooks";
import {
  SubjectSelect,
  TextbookSelect,
  RangeUnitSelect,
  NumberStepper,
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

const todayStr = () => new Date().toISOString().slice(0, 10);

// クイック加算ボタンの候補（分）
const QUICK_MINUTES = [15, 30, 60];

// 空のフォーム初期値（日付は今日、時間は未入力）
const emptyValues = () => ({
  date: todayStr(),
  subject: null,
  textbookId: null,
  rangeStart: null,
  rangeEnd: null,
  rangeUnit: null,
  memo: "",
});

export default function StudyLogForm() {
  const createLog = useCreateStudyLog();
  const { data: textbooks = [] } = useTextbooks();

  const form = useForm<CreateStudyLogInput>({
    resolver: zodResolver(createStudyLogSchema),
    defaultValues: emptyValues(),
  });
  const selectedTextbookId = useWatch({
    control: form.control,
    name: "textbookId",
  });
  const selectedTextbook = textbooks.find(
    (textbook) => textbook.id === selectedTextbookId
  );
  const lockedRangeUnit = selectedTextbook?.rangeUnit ?? null;

  const onSubmit = (data: CreateStudyLogInput) => {
    createLog.mutate(data, {
      onSuccess: () => {
        toast.success("学習実績を記録しました");
        // 日付は保ちつつ、他は空に戻して続けて記録できるようにする
        form.reset({ ...emptyValues(), date: form.getValues("date") });
      },
      onError: (error) => toast.error(error.message),
    });
  };

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-3">
        {/* 上段：日付・科目・学習時間（必須） */}
        <div className="flex flex-wrap items-end gap-3">
          <FormField
            control={form.control}
            name="date"
            render={({ field }) => (
              <FormItem>
                <FormLabel>日付</FormLabel>
                <FormControl>
                  <Input type="date" className="h-9 w-40" {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
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
            control={form.control}
            name="minutes"
            render={({ field }) => (
              <FormItem>
                <FormLabel>学習時間（分）</FormLabel>
                <FormControl>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      min={1}
                      placeholder="分"
                      className="h-9 w-20 [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
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
                        onClick={() =>
                          field.onChange((field.value ?? 0) + m)
                        }
                      >
                        +{m}
                      </Button>
                    ))}
                  </div>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        {/* 中段：任意で参考書＋範囲 */}
        <div className="flex flex-wrap items-center gap-2">
          <FormField
            control={form.control}
            name="textbookId"
            render={({ field }) => (
              <TextbookSelect
                value={field.value}
                onChange={(textbookId) => {
                  field.onChange(textbookId);
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
          />
          <FormField
            control={form.control}
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
            control={form.control}
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
            control={form.control}
            name="rangeUnit"
            render={({ field }) => (
              <FormItem>
                <FormControl>
                  <RangeUnitSelect
                    value={field.value}
                    onChange={field.onChange}
                    disabled={lockedRangeUnit != null}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        {/* 下段：任意メモ ＋ 記録ボタン */}
        <FormField
          control={form.control}
          name="memo"
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
        <div className="flex justify-end">
          <Button type="submit" disabled={createLog.isPending}>
            記録する
          </Button>
        </div>
      </form>
    </Form>
  );
}
