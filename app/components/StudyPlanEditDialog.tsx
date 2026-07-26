"use client";

import { useState } from "react";
import type { StudyPlan } from "@/app/hooks/useStudyPlans";
import { useTextbooks, useUpdateTextbookSubject } from "@/app/hooks/useTextbooks";
import type { UpdateStudyPlanInput } from "@/lib/validations/studyPlan";
import { RANGE_UNITS } from "@/lib/validations/studyPlan";
import { SUBJECTS, subjectColor, subjectLabel } from "@/lib/subjects";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

function naturalDate(ymd: string): string {
  const [year, month, day] = ymd.split("-").map(Number);
  const value = new Date(year, month - 1, day);
  const weekday = ["日", "月", "火", "水", "木", "金", "土"][value.getDay()];
  return `${month}月${day}日（${weekday}）`;
}

export default function StudyPlanEditDialog({
  plan,
  saving,
  saveError,
  onSave,
  onClose,
}: {
  plan: StudyPlan;
  saving: boolean;
  saveError: string | null;
  onSave: (data: UpdateStudyPlanInput) => void;
  onClose: () => void;
}) {
  const { data: textbooks = [], isPending, isError, refetch } = useTextbooks();
  const updateTextbookSubject = useUpdateTextbookSubject();
  const isTextbookPlan = plan.textbookId != null;
  const [date, setDate] = useState(plan.date.slice(0, 10));
  const [textbookId, setTextbookId] = useState(plan.textbookId);
  const [rangeStart, setRangeStart] = useState(plan.rangeStart?.toString() ?? "");
  const [rangeEnd, setRangeEnd] = useState(plan.rangeEnd?.toString() ?? "");
  const [content, setContent] = useState(plan.content ?? "");
  const [subject, setSubject] = useState(plan.subject);
  const [formError, setFormError] = useState<string | null>(null);

  const selectedTextbook = textbooks.find((textbook) => textbook.id === textbookId);
  const textbookChanged = textbookId !== plan.textbookId;
  const effectiveRangeUnit = textbookChanged
    ? selectedTextbook?.rangeUnit ?? "page"
    : plan.rangeUnit ?? selectedTextbook?.rangeUnit ?? "page";
  const original = {
    date: plan.date.slice(0, 10),
    textbookId: plan.textbookId,
    rangeStart: plan.rangeStart?.toString() ?? "",
    rangeEnd: plan.rangeEnd?.toString() ?? "",
    content: plan.content ?? "",
    subject: plan.subject,
  };
  // 参考書予定の科目は参考書に紐づく。参考書側の科目を変えたら「保存」で
  // この予定にも反映できるよう、差分として検知する。
  const textbookSubjectChanged =
    isTextbookPlan &&
    selectedTextbook != null &&
    selectedTextbook.subject !== plan.subject;
  const isDirty =
    date !== original.date ||
    textbookId !== original.textbookId ||
    rangeStart !== original.rangeStart ||
    rangeEnd !== original.rangeEnd ||
    content !== original.content ||
    subject !== original.subject ||
    textbookSubjectChanged;

  const requestClose = () => {
    if (isDirty && !window.confirm("変更内容を破棄して閉じますか？")) return;
    onClose();
  };

  const submit = () => {
    setFormError(null);
    if (!date) {
      setFormError("日付を選んでください");
      return;
    }
    if (isTextbookPlan && (!textbookId || !selectedTextbook)) {
      setFormError("参考書を選んでください");
      return;
    }
    if (!isTextbookPlan && !content.trim()) {
      setFormError("勉強する内容を入力してください");
      return;
    }
    const hasStart = rangeStart !== "";
    const hasEnd = rangeEnd !== "";
    if (isTextbookPlan && hasStart !== hasEnd) {
      setFormError("範囲は開始と終了の両方を入力してください");
      return;
    }
    if (isTextbookPlan && hasStart && Number(rangeStart) > Number(rangeEnd)) {
      setFormError("終了は開始以上にしてください");
      return;
    }

    onSave(
      isTextbookPlan && selectedTextbook
        ? {
            date,
            textbookId: selectedTextbook.id,
            // 科目は参考書に紐づくため、常に参考書の現在の科目へ揃える
            subject: selectedTextbook.subject,
            rangeStart: hasStart ? Number(rangeStart) : null,
            rangeEnd: hasEnd ? Number(rangeEnd) : null,
            rangeUnit: hasStart ? effectiveRangeUnit : null,
            content: content.trim(),
          }
        : {
            date,
            textbookId: null,
            subject,
            rangeStart: null,
            rangeEnd: null,
            rangeUnit: null,
            content: content.trim(),
          }
    );
  };

  return (
    <Dialog open onOpenChange={(open) => !open && requestClose()}>
      <DialogContent className="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-md">
        <DialogHeader className="border-b px-5 py-4 pr-12">
          <DialogTitle>学習予定を編集</DialogTitle>
          <DialogDescription>
            {date ? `${naturalDate(date)}の予定内容を変更します。` : "予定内容を変更します。"}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-5">
          <div>
            <Label htmlFor="edit-plan-date">日付</Label>
            <Input
              id="edit-plan-date"
              type="date"
              className="mt-2"
              value={date}
              onChange={(event) => setDate(event.target.value)}
            />
          </div>

          {isTextbookPlan ? (
            <>
              <div>
                <Label className="mb-2 block">参考書</Label>
                {isPending ? (
                  <div className="rounded-lg bg-muted/50 p-4 text-sm text-muted-foreground" role="status">
                    参考書を読み込んでいます…
                  </div>
                ) : isError ? (
                  <div className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                    <p>参考書を読み込めませんでした。</p>
                    <button type="button" onClick={() => refetch()} className="mt-2 font-medium underline">
                      再試行
                    </button>
                  </div>
                ) : (
                  <fieldset className="space-y-2">
                    <legend className="sr-only">参考書を選択</legend>
                    {textbooks.map((textbook) => {
                      const selected = textbookId === textbook.id;
                      return (
                        <label
                          key={textbook.id}
                          className={`relative flex min-h-14 w-full cursor-pointer items-center gap-3 rounded-lg border px-3 py-3 text-left has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring ${selected ? "border-primary bg-info/10" : "hover:border-muted-foreground"}`}
                        >
                          <input
                            type="radio"
                            name="edit-plan-textbook"
                            value={textbook.id}
                            checked={selected}
                            onChange={() => {
                              if (textbookId === textbook.id) return;
                              setTextbookId(textbook.id);
                              setRangeStart("");
                              setRangeEnd("");
                            }}
                            className="sr-only"
                          />
                          <span className={`h-4 w-4 shrink-0 rounded-full border-2 ${selected ? "border-[5px] border-primary" : "border-border"}`} />
                          <span className="min-w-0">
                            <span className="block truncate font-medium text-foreground">{textbook.name}</span>
                            <span className="mt-0.5 block text-xs text-muted-foreground">{subjectLabel(textbook.subject)}</span>
                          </span>
                        </label>
                      );
                    })}
                  </fieldset>
                )}
              </div>

              {selectedTextbook ? (
                <>
                  <div>
                    <Label className="mb-2 block">学習範囲（任意）</Label>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <Label htmlFor="edit-plan-range-start" className="text-xs text-muted-foreground">開始</Label>
                        <Input id="edit-plan-range-start" type="number" min={1} value={rangeStart} onChange={(event) => setRangeStart(event.target.value)} />
                      </div>
                      <div>
                        <Label htmlFor="edit-plan-range-end" className="text-xs text-muted-foreground">終了</Label>
                        <Input id="edit-plan-range-end" type="number" min={1} value={rangeEnd} onChange={(event) => setRangeEnd(event.target.value)} />
                      </div>
                    </div>
                    <p className="mt-2 text-xs text-muted-foreground">
                      単位：{RANGE_UNITS.find((unit) => unit.value === effectiveRangeUnit)?.label ?? "ページ"}
                    </p>
                  </div>
                  <div>
                    <Label className="mb-2 block">科目（任意）</Label>
                    <div className="flex flex-wrap gap-2">
                      {SUBJECTS.map((item) => {
                        const selected = selectedTextbook.subject === item.value;
                        return (
                          <button
                            key={item.value}
                            type="button"
                            aria-pressed={selected}
                            disabled={updateTextbookSubject.isPending}
                            onClick={() =>
                              updateTextbookSubject.mutate({
                                id: selectedTextbook.id,
                                subject: selected ? null : item.value,
                              })
                            }
                            className="min-h-10 rounded-full border bg-card px-3 text-sm font-medium text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60"
                            style={selected ? { borderColor: subjectColor(item.value), color: subjectColor(item.value), backgroundColor: `${subjectColor(item.value)}12` } : undefined}
                          >
                            {item.label}
                          </button>
                        );
                      })}
                    </div>
                    <p className="mt-2 text-xs text-muted-foreground">
                      科目は参考書ごとに設定され、この参考書を使う予定・記録にも反映されます。
                    </p>
                  </div>
                  <div>
                    <Label htmlFor="edit-plan-memo">メモ（任意）</Label>
                    <Input id="edit-plan-memo" className="mt-2" value={content} onChange={(event) => setContent(event.target.value)} maxLength={500} />
                  </div>
                </>
              ) : null}
            </>
          ) : (
            <>
              <div>
                <Label htmlFor="edit-plan-content">学習内容</Label>
                <Input id="edit-plan-content" className="mt-2" value={content} onChange={(event) => setContent(event.target.value)} maxLength={500} />
              </div>
              <div>
                <Label className="mb-2 block">科目（任意）</Label>
                <div className="flex flex-wrap gap-2">
                  {SUBJECTS.map((item) => {
                    const selected = subject === item.value;
                    return (
                      <button
                        key={item.value}
                        type="button"
                        aria-pressed={selected}
                        onClick={() => setSubject(selected ? null : item.value)}
                        className="min-h-10 rounded-full border bg-card px-3 text-sm font-medium text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        style={selected ? { borderColor: subjectColor(item.value), color: subjectColor(item.value), backgroundColor: `${subjectColor(item.value)}12` } : undefined}
                      >
                        {item.label}
                      </button>
                    );
                  })}
                </div>
              </div>
            </>
          )}

          {formError ? (
            <div role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {formError}
            </div>
          ) : null}
          {saveError ? (
            <div role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {saveError}
            </div>
          ) : null}
        </div>

        <div className="border-t bg-card px-5 py-4">
          <Button type="button" className="min-h-11 w-full" onClick={submit} disabled={saving || !isDirty || (isTextbookPlan && !selectedTextbook)}>
            {saving ? "保存中…" : "変更を保存"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
