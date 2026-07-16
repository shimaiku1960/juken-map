"use client";

import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import {
  createStudyLogSchema,
  type CreateStudyLogInput,
} from "@/lib/validations/studyLog";
import { useCreateStudyLog } from "@/app/hooks/useStudyLogs";
import {
  useTextbooks,
  useTextbookMasters,
  useUpdateTextbookProgress,
} from "@/app/hooks/useTextbooks";
import { todayYmd, ymdLocal } from "@/lib/date";
import { RANGE_UNIT_VALUES } from "@/lib/validations/studyPlan";
import type { UpdateTextbookProgressInput } from "@/lib/validations/textbook";
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

// クイック加算ボタンの候補（分）
const QUICK_MINUTES = [15, 30, 60];

// 空のフォーム初期値（日付は今日、時間は未入力）
const emptyValues = () => ({
  date: todayYmd(),
  subject: null,
  textbookId: null,
  rangeStart: null,
  rangeEnd: null,
  rangeUnit: null,
  memo: "",
});

export default function StudyLogForm({
  variant = "full",
  onSuccess,
}: {
  variant?: "full" | "quick";
  onSuccess?: () => void;
}) {
  const createLog = useCreateStudyLog();
  const { data: textbooks = [] } = useTextbooks();
  const { data: masters = [] } = useTextbookMasters();
  const updateProgress = useUpdateTextbookProgress();

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

  // 逆算設定済みの参考書を選んで単位を変えたら、その参考書の「追跡単位」を切り替える。
  // マスタ参考書ならその単位の総量も自動反映。1参考書=1単位で逆算の一貫性を保つ。
  const applyUnitToTextbook = (unit: string | null) => {
    if (
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
        rangeUnit: unit as UpdateTextbookProgressInput["rangeUnit"],
        targetDate: selectedTextbook.targetDate
          ? ymdLocal(selectedTextbook.targetDate)
          : null,
      },
    });
  };

  const onSubmit = (data: CreateStudyLogInput) => {
    createLog.mutate(data, {
      onSuccess: () => {
        toast.success("学習実績を記録しました");
        // 日付は保ちつつ、他は空に戻して続けて記録できるようにする
        form.reset({ ...emptyValues(), date: form.getValues("date") });
        onSuccess?.();
      },
      onError: (error) => toast.error(error.message),
    });
  };

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-3">
        {/* 最初に毎日使う項目だけを見せる。クイック記録では日付は今日を既定にする。 */}
        <div className="flex flex-wrap items-end gap-3">
          {variant === "full" && (
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
          )}
          <FormField
            control={form.control}
            name="subject"
            render={({ field }) => (
              <FormItem>
                <FormLabel htmlFor={`study-log-subject-${variant}`}>
                  科目
                </FormLabel>
                <FormControl>
                  <div>
                    <SubjectSelect
                      id={`study-log-subject-${variant}`}
                      ariaLabel="科目"
                      className={variant === "quick" ? "h-11" : undefined}
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
        </div>

        {variant === "quick" && (
          <p className="text-sm text-muted-foreground">
            今日の実績として記録します。科目は未選択でも保存できます。
          </p>
        )}

        <details
          open={variant === "full" ? true : undefined}
          className={variant === "quick" ? "rounded-lg border p-3" : "contents"}
        >
          {variant === "quick" && (
            <summary className="cursor-pointer text-sm font-medium">
              日付・参考書・範囲・メモを追加
            </summary>
          )}
          <div
            className={
              variant === "quick" ? "mt-4 space-y-3" : "space-y-3"
            }
          >
            {variant === "quick" && (
              <FormField
                control={form.control}
                name="date"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>日付</FormLabel>
                    <FormControl>
                      <Input type="date" className="h-11 w-44" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {/* 任意で参考書＋範囲 */}
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

            {/* 任意メモ */}
            <FormField
              control={form.control}
              name="memo"
              render={({ field }) => (
                <FormItem>
                  <FormControl>
                    <Input
                      placeholder="メモ（任意）"
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
        <div className="flex justify-end">
          <Button
            type="submit"
            className={
              variant === "quick" ? "h-11 w-full sm:w-auto" : undefined
            }
            disabled={createLog.isPending}
          >
            {createLog.isPending ? "記録中..." : "記録する"}
          </Button>
        </div>
      </form>
    </Form>
  );
}
