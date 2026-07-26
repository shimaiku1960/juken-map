"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, BookOpen, Check, PencilLine } from "lucide-react";
import { toast } from "sonner";
import { studyPlansKey } from "@/app/hooks/useStudyPlans";
import { useCreateTextbook, useTextbooks } from "@/app/hooks/useTextbooks";
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

type View = "choose" | "textbook" | "free" | "new-textbook" | "success";

type CreatedSummary = {
  subject: string | null;
  title: string;
  range: string | null;
  memo: string | null;
};

function naturalDate(ymd: string): string {
  const [year, month, day] = ymd.split("-").map(Number);
  const date = new Date(year, month - 1, day);
  const weekday = ["日", "月", "火", "水", "木", "金", "土"][date.getDay()];
  return `${month}月${day}日（${weekday}）`;
}

function rangeUnitLabel(value: string | null): string {
  return RANGE_UNITS.find((unit) => unit.value === value)?.label ?? "ページ";
}

async function responseError(response: Response): Promise<string> {
  const body = await response.json().catch(() => null);
  if (typeof body?.error === "string") return body.error;
  if (Array.isArray(body?.error) && typeof body.error[0]?.message === "string") {
    return body.error[0].message;
  }
  return "予定を追加できませんでした";
}

export default function StudyPlanCreateDialog({
  date,
  onClose,
}: {
  date: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const {
    data: textbooks = [],
    isPending: textbooksPending,
    isError: textbooksError,
    refetch: refetchTextbooks,
  } = useTextbooks();
  const createTextbook = useCreateTextbook();
  const [view, setView] = useState<View>("choose");
  const [selectedTextbookId, setSelectedTextbookId] = useState<number | null>(null);
  const [rangeStart, setRangeStart] = useState("");
  const [rangeEnd, setRangeEnd] = useState("");
  const [memo, setMemo] = useState("");
  const [freeContent, setFreeContent] = useState("");
  const [freeSubject, setFreeSubject] = useState<string | null>(null);
  const [newTextbookName, setNewTextbookName] = useState("");
  const [newTextbookSubject, setNewTextbookSubject] = useState<string | null>(null);
  const [newTextbookUnit, setNewTextbookUnit] = useState<
    (typeof RANGE_UNITS)[number]["value"]
  >("page");
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [created, setCreated] = useState<CreatedSummary | null>(null);

  const selectedTextbook = textbooks.find(
    (textbook) => textbook.id === selectedTextbookId
  );
  const isDirty =
    selectedTextbookId != null ||
    rangeStart !== "" ||
    rangeEnd !== "" ||
    memo.trim() !== "" ||
    freeContent.trim() !== "" ||
    freeSubject != null ||
    newTextbookName.trim() !== "" ||
    newTextbookSubject != null ||
    newTextbookUnit !== "page";

  const requestClose = () => {
    if (
      view !== "success" &&
      isDirty &&
      !window.confirm("入力内容を破棄して閉じますか？")
    ) {
      return;
    }
    onClose();
  };

  const resetForNext = () => {
    setView("choose");
    setSelectedTextbookId(null);
    setRangeStart("");
    setRangeEnd("");
    setMemo("");
    setFreeContent("");
    setFreeSubject(null);
    setNewTextbookName("");
    setNewTextbookSubject(null);
    setNewTextbookUnit("page");
    setFormError(null);
    setCreated(null);
  };

  const submit = async () => {
    setFormError(null);

    if (view === "textbook" && !selectedTextbook) {
      setFormError("参考書を選んでください");
      return;
    }
    if (view === "free" && freeContent.trim() === "") {
      setFormError("勉強する内容を入力してください");
      return;
    }

    const hasStart = rangeStart !== "";
    const hasEnd = rangeEnd !== "";
    if (view === "textbook" && hasStart !== hasEnd) {
      setFormError("範囲は開始と終了の両方を入力してください");
      return;
    }
    if (
      view === "textbook" &&
      hasStart &&
      Number(rangeStart) > Number(rangeEnd)
    ) {
      setFormError("終了は開始以上にしてください");
      return;
    }

    const unit = selectedTextbook?.rangeUnit ?? "page";
    const item =
      view === "textbook" && selectedTextbook
        ? {
            textbookId: selectedTextbook.id,
            subject: selectedTextbook.subject,
            rangeStart: hasStart ? Number(rangeStart) : null,
            rangeEnd: hasEnd ? Number(rangeEnd) : null,
            rangeUnit: hasStart ? unit : null,
            content: memo.trim(),
          }
        : {
            textbookId: null,
            subject: freeSubject,
            rangeStart: null,
            rangeEnd: null,
            rangeUnit: null,
            content: freeContent.trim(),
          };

    setSaving(true);
    try {
      const response = await fetch("/api/study-plans", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ date, items: [item] }),
      });
      if (!response.ok) throw new Error(await responseError(response));

      await queryClient.invalidateQueries({ queryKey: studyPlansKey });
      const range =
        view === "textbook" && hasStart
          ? `${rangeStart}〜${rangeEnd}${rangeUnitLabel(unit)}`
          : null;
      setCreated({
        subject: item.subject,
        title: selectedTextbook?.name ?? freeContent.trim(),
        range,
        memo: view === "textbook" && memo.trim() ? memo.trim() : null,
      });
      setView("success");
      toast.success("学習予定を追加しました");
    } catch (error) {
      setFormError(
        error instanceof Error ? error.message : "予定を追加できませんでした"
      );
    } finally {
      setSaving(false);
    }
  };

  const addNewTextbook = () => {
    const name = newTextbookName.trim();
    if (!name) {
      setFormError("参考書名を入力してください");
      return;
    }
    setFormError(null);
    createTextbook.mutate(
      { name, subject: newTextbookSubject, rangeUnit: newTextbookUnit },
      {
        onSuccess: (textbook) => {
          setSelectedTextbookId(textbook.id);
          setNewTextbookName("");
          setView("textbook");
        },
        onError: (error) => setFormError(error.message),
      }
    );
  };

  const showBack = view !== "choose" && view !== "success";

  return (
    <Dialog open onOpenChange={(open) => !open && requestClose()}>
      <DialogContent className="top-auto bottom-0 flex max-h-[90dvh] max-w-none translate-y-0 flex-col gap-0 overflow-hidden rounded-b-none p-0 sm:top-1/2 sm:bottom-auto sm:max-w-md sm:-translate-y-1/2 sm:rounded-xl">
        <DialogHeader className="border-b px-5 py-4 pr-12">
          {showBack ? (
            <button
              type="button"
              onClick={() => {
                setFormError(null);
                setView("choose");
              }}
              className="mb-2 inline-flex w-fit items-center gap-1 text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <ArrowLeft className="h-4 w-4" /> 戻る
            </button>
          ) : null}
          <DialogTitle>
            {view === "success"
              ? "予定を追加しました"
              : `${naturalDate(date)}の予定を追加`}
          </DialogTitle>
          <DialogDescription>
            {view === "success"
              ? "追加した予定を確認し、続けて追加するか完了できます。"
              : "この日に勉強することを1つ決めます。"}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
          {view === "choose" ? (
            <div className="space-y-3">
              <p className="mb-4 text-base font-semibold text-foreground">
                何を勉強する？
              </p>
              <button
                type="button"
                onClick={() => setView("textbook")}
                className="flex min-h-20 w-full items-center gap-4 rounded-xl border p-4 text-left transition hover:border-primary/60 hover:bg-info/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="rounded-lg bg-info/15 p-2 text-primary">
                  <BookOpen className="h-5 w-5" />
                </span>
                <span>
                  <span className="block font-semibold text-foreground">参考書から選ぶ</span>
                  <span className="mt-1 block text-sm text-muted-foreground">
                    登録済みの教材と範囲を指定
                  </span>
                </span>
              </button>
              <button
                type="button"
                onClick={() => setView("free")}
                className="flex min-h-20 w-full items-center gap-4 rounded-xl border p-4 text-left transition hover:border-primary/60 hover:bg-info/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="rounded-lg bg-muted p-2 text-foreground">
                  <PencilLine className="h-5 w-5" />
                </span>
                <span>
                  <span className="block font-semibold text-foreground">自由に入力する</span>
                  <span className="mt-1 block text-sm text-muted-foreground">
                    復習、過去問、授業など
                  </span>
                </span>
              </button>
            </div>
          ) : null}

          {view === "textbook" ? (
            <div className="space-y-5">
              <div>
                <Label className="mb-2 block">参考書</Label>
                {textbooksPending ? (
                  <div className="rounded-lg bg-muted/50 p-4 text-sm text-muted-foreground" role="status">
                    参考書を読み込んでいます…
                  </div>
                ) : textbooksError ? (
                  <div className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                    <p>参考書を読み込めませんでした。</p>
                    <button type="button" onClick={() => refetchTextbooks()} className="mt-2 font-medium underline">
                      再試行
                    </button>
                  </div>
                ) : textbooks.length === 0 ? (
                  <div className="rounded-lg bg-muted/50 p-4 text-sm text-muted-foreground">
                    <p>登録済みの参考書がありません。</p>
                    <Button type="button" size="sm" variant="outline" className="mt-3" onClick={() => setView("new-textbook")}>参考書を登録する</Button>
                  </div>
                ) : (
                  <div className="space-y-2" aria-label="参考書を選択">
                    {textbooks.map((textbook) => {
                      const selected = selectedTextbookId === textbook.id;
                      return (
                        <button
                          key={textbook.id}
                          type="button"
                          aria-pressed={selected}
                          onClick={() => {
                            setSelectedTextbookId(textbook.id);
                            setRangeStart("");
                            setRangeEnd("");
                          }}
                          className={`flex min-h-14 w-full items-center gap-3 rounded-lg border px-3 py-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${selected ? "border-primary bg-info/10" : "hover:border-muted-foreground"}`}
                        >
                          <span className={`h-4 w-4 shrink-0 rounded-full border-2 ${selected ? "border-[5px] border-primary" : "border-border"}`} />
                          <span className="min-w-0">
                            <span className="block truncate font-medium text-foreground">{textbook.name}</span>
                            <span className="mt-0.5 block text-xs text-muted-foreground">{subjectLabel(textbook.subject)}</span>
                          </span>
                        </button>
                      );
                    })}
                    <button type="button" onClick={() => setView("new-textbook")} className="mt-2 text-sm font-medium text-primary hover:underline">＋ 新しい参考書を登録</button>
                  </div>
                )}
              </div>

              {selectedTextbook ? (
                <>
                  <div>
                    <Label className="mb-2 block">学習範囲（任意）</Label>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <Label htmlFor="plan-range-start" className="text-xs text-muted-foreground">開始</Label>
                        <Input id="plan-range-start" type="number" min={1} value={rangeStart} onChange={(event) => setRangeStart(event.target.value)} placeholder="例：32" />
                      </div>
                      <div>
                        <Label htmlFor="plan-range-end" className="text-xs text-muted-foreground">終了</Label>
                        <Input id="plan-range-end" type="number" min={1} value={rangeEnd} onChange={(event) => setRangeEnd(event.target.value)} placeholder="例：45" />
                      </div>
                    </div>
                    <p className="mt-2 text-xs text-muted-foreground">単位：{rangeUnitLabel(selectedTextbook.rangeUnit)}</p>
                  </div>
                  <div>
                    <Label htmlFor="plan-memo">メモ（任意）</Label>
                    <Input id="plan-memo" className="mt-2" value={memo} onChange={(event) => setMemo(event.target.value)} placeholder="例：例題を中心に進める" maxLength={500} />
                  </div>
                </>
              ) : null}
            </div>
          ) : null}

          {view === "free" ? (
            <div className="space-y-5">
              <div>
                <Label htmlFor="plan-free-content">学習内容</Label>
                <Input id="plan-free-content" autoFocus className="mt-2" value={freeContent} onChange={(event) => setFreeContent(event.target.value)} placeholder="例：英文法の復習" maxLength={500} />
              </div>
              <div>
                <Label className="mb-2 block">科目（任意）</Label>
                <div className="flex flex-wrap gap-2">
                  {SUBJECTS.map((subject) => {
                    const selected = freeSubject === subject.value;
                    return (
                      <button
                        key={subject.value}
                        type="button"
                        aria-pressed={selected}
                        onClick={() => setFreeSubject(selected ? null : subject.value)}
                        className={`min-h-10 rounded-full border bg-card px-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${selected ? "ring-1" : "text-foreground"}`}
                        style={
                          selected
                            ? {
                                borderColor: subjectColor(subject.value),
                                color: subjectColor(subject.value),
                                backgroundColor: `${subjectColor(subject.value)}12`,
                              }
                            : undefined
                        }
                      >
                        {subject.label}
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>
          ) : null}

          {view === "new-textbook" ? (
            <div className="space-y-5">
              <div>
                <Label htmlFor="new-textbook-name">参考書名</Label>
                <Input id="new-textbook-name" autoFocus className="mt-2" value={newTextbookName} onChange={(event) => setNewTextbookName(event.target.value)} placeholder="例：青チャートIA" maxLength={100} />
              </div>
              <div>
                <Label className="mb-2 block">科目（任意）</Label>
                <div className="flex flex-wrap gap-2">
                  {SUBJECTS.map((subject) => (
                    <button
                      key={subject.value}
                      type="button"
                      aria-pressed={newTextbookSubject === subject.value}
                      onClick={() => setNewTextbookSubject(newTextbookSubject === subject.value ? null : subject.value)}
                      className="min-h-10 rounded-full border px-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      style={newTextbookSubject === subject.value ? { borderColor: subjectColor(subject.value), color: subjectColor(subject.value), backgroundColor: `${subjectColor(subject.value)}12` } : undefined}
                    >
                      {subject.label}
                    </button>
                  ))}
                </div>
              </div>
              <div>
                <Label htmlFor="new-textbook-unit">範囲の単位</Label>
                <select
                  id="new-textbook-unit"
                  value={newTextbookUnit}
                  onChange={(event) =>
                    setNewTextbookUnit(
                      event.target.value as (typeof RANGE_UNITS)[number]["value"]
                    )
                  }
                  className="mt-2 h-10 w-full rounded-lg border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {RANGE_UNITS.map((unit) => <option key={unit.value} value={unit.value}>{unit.label}</option>)}
                </select>
              </div>
              <Button type="button" className="w-full" onClick={addNewTextbook} disabled={createTextbook.isPending}>
                {createTextbook.isPending ? "登録中…" : "参考書を登録して選択"}
              </Button>
            </div>
          ) : null}

          {view === "success" && created ? (
            <div className="space-y-5 text-center">
              <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-success/15 text-success"><Check className="h-6 w-6" /></span>
              <div className="rounded-xl border bg-muted/50 p-4 text-left">
                <p className="mb-2 text-sm font-medium text-foreground">{naturalDate(date)}</p>
                <p className="text-xs text-muted-foreground">{subjectLabel(created.subject)}</p>
                <p className="mt-1 font-semibold text-foreground">{created.title}</p>
                {created.range ? <p className="mt-1 text-sm text-muted-foreground">{created.range}</p> : null}
                {created.memo ? <p className="mt-2 text-sm text-muted-foreground">メモ：{created.memo}</p> : null}
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                <Button type="button" variant="outline" onClick={resetForNext}>続けて予定を追加</Button>
                <Button type="button" onClick={onClose}>完了</Button>
              </div>
            </div>
          ) : null}

          {formError ? (
            <div role="alert" className="mt-4 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {formError}
            </div>
          ) : null}
        </div>

        {(view === "textbook" && selectedTextbook) || view === "free" ? (
          <div className="border-t bg-card px-5 pt-4 pb-[calc(1rem+env(safe-area-inset-bottom))] sm:pb-4">
            <Button type="button" className="min-h-11 w-full" onClick={submit} disabled={saving}>
              {saving ? "追加中…" : "この予定を追加"}
            </Button>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
