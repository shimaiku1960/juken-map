import { useState } from "react";
import { ArrowLeft, BookOpen, PencilLine, Play } from "lucide-react";
import { useCreateTextbook, useTextbooks } from "@/web/hooks/useTextbooks";
import { SUBJECTS, subjectColor } from "@/shared/subjects";
import { RANGE_UNITS } from "@/shared/validations/studyPlan";
import type { StudySessionTarget } from "@/web/lib/studySession";
import { Button } from "@/web/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/web/components/ui/dialog";
import { Input } from "@/web/components/ui/input";
import { Label } from "@/web/components/ui/label";

type PickerView = "choose" | "textbook" | "free" | "new-textbook";

// 「何を勉強するか」を決めるのに必要な分だけ。予定の完了状態などは親の関心事。
export type SelectablePlan = {
  id: number;
  content: string;
  subject: string | null;
  textbookId: number | null;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
};

/**
 * 計測を始める前の「何を勉強しますか？」ダイアログ。
 *
 * 選びかけの状態（どの画面か・どれを選んだか・新しい参考書の入力）はすべて
 * この中に閉じている。開くたびに作り直したいので、親は開くときに key を
 * 変えて再マウントさせること。そうすれば前回の選択が残らない。
 *
 * 参考書の一覧と登録もここでしか使わないため、フックもここで呼ぶ。
 */
export default function StudySessionPickerDialog({
  open,
  onOpenChange,
  selectablePlans,
  onStart,
  onManualLog,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectablePlans: SelectablePlan[];
  /** 選び終わったものを計測対象として親へ渡す */
  onStart: (target: StudySessionTarget) => void;
  /** 「あとから記録」へ切り替える */
  onManualLog: () => void;
}) {
  const {
    data: textbooks = [],
    isPending: textbooksPending,
    isError: textbooksError,
    refetch: refetchTextbooks,
  } = useTextbooks();
  const createTextbook = useCreateTextbook();

  const [view, setView] = useState<PickerView>("choose");
  const [error, setError] = useState<string | null>(null);
  const [selectedTarget, setSelectedTarget] = useState(() =>
    selectablePlans[0] ? `plan:${selectablePlans[0].id}` : ""
  );
  const [manualLabel, setManualLabel] = useState("");
  const [manualSubject, setManualSubject] = useState<string | null>(null);
  const [manualTextbookId, setManualTextbookId] = useState<number | null>(null);
  const [newTextbookName, setNewTextbookName] = useState("");
  const [newTextbookSubject, setNewTextbookSubject] = useState<string | null>(
    null
  );
  const [newTextbookUnit, setNewTextbookUnit] = useState<
    (typeof RANGE_UNITS)[number]["value"]
  >("page");

  const addNewTextbook = () => {
    const name = newTextbookName.trim();
    if (!name) {
      setError("参考書名を入力してください");
      return;
    }
    setError(null);
    createTextbook.mutate(
      { name, subject: newTextbookSubject, rangeUnit: newTextbookUnit },
      {
        onSuccess: (textbook) => {
          setManualTextbookId(textbook.id);
          setNewTextbookName("");
          setView("textbook");
        },
        onError: (cause) => setError(cause.message),
      }
    );
  };

  // 選んだ内容を計測対象に組み立てる。足りなければここで止めて理由を出す。
  const start = () => {
    if (selectedTarget.startsWith("plan:")) {
      const planId = Number(selectedTarget.slice("plan:".length));
      const plan = selectablePlans.find((candidate) => candidate.id === planId);
      if (!plan) return;
      onStart({
        planId: plan.id,
        label: plan.content,
        subject: plan.subject,
        textbookId: plan.textbookId,
        rangeStart: plan.rangeStart,
        rangeEnd: plan.rangeEnd,
        rangeUnit: plan.rangeUnit,
      });
      return;
    }

    if (view === "textbook") {
      const textbook = textbooks.find((item) => item.id === manualTextbookId);
      if (!textbook) {
        setError("参考書を選んでください");
        return;
      }
      onStart({
        planId: null,
        label: textbook.name,
        subject: textbook.subject,
        textbookId: textbook.id,
        rangeStart: null,
        rangeEnd: null,
        rangeUnit: textbook?.rangeUnit ?? null,
      });
      return;
    }

    if (view === "free") {
      const trimmedLabel = manualLabel.trim();
      if (!trimmedLabel) {
        setError("勉強する内容を入力してください");
        return;
      }
      onStart({
        planId: null,
        label: trimmedLabel,
        subject: manualSubject,
        textbookId: null,
        rangeStart: null,
        rangeEnd: null,
        rangeUnit: null,
      });
      return;
    }

    setError("勉強する内容を選んでください");
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="top-auto bottom-0 flex max-h-[90dvh] max-w-none translate-y-0 flex-col gap-0 overflow-hidden rounded-b-none p-0 sm:top-1/2 sm:bottom-auto sm:max-w-md sm:-translate-y-1/2 sm:rounded-xl">
        <DialogHeader className="border-b px-5 py-4 pr-12">
          {view !== "choose" ? (
            <button
              type="button"
              onClick={() => {
                setError(null);
                if (view === "new-textbook") {
                  setView("textbook");
                } else {
                  setView("choose");
                  setSelectedTarget(
                    selectablePlans[0] ? `plan:${selectablePlans[0].id}` : ""
                  );
                }
              }}
              className="mb-2 inline-flex w-fit items-center gap-1 text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              戻る
            </button>
          ) : null}
          <DialogTitle>
            {view === "textbook"
              ? "参考書から選ぶ"
              : view === "free"
                ? "自由に入力する"
                : view === "new-textbook"
                  ? "新しい参考書を登録"
                  : "何を勉強しますか？"}
          </DialogTitle>
          <DialogDescription>
            {view === "choose"
              ? "今日の予定を選ぶか、学習内容を選びます。"
              : view === "new-textbook"
                ? "登録後、その参考書を選んだ状態に戻ります。"
                : "勉強する内容を決めて、時間計測を始めます。"}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
          {view === "choose" ? (
            <div className="space-y-3">
              {selectablePlans.length > 0 && (
                <fieldset className="space-y-2">
                  <legend className="text-sm font-medium">今日の予定</legend>
                  {selectablePlans.map((plan) => (
                    <label
                      key={plan.id}
                      className="flex min-h-12 cursor-pointer gap-3 rounded-lg border p-3 has-[:checked]:border-primary has-[:checked]:bg-info/10"
                    >
                      <input
                        type="radio"
                        name="study-target"
                        value={`plan:${plan.id}`}
                        checked={selectedTarget === `plan:${plan.id}`}
                        onChange={(event) =>
                          setSelectedTarget(event.target.value)
                        }
                      />
                      <span className="min-w-0 font-medium">
                        {plan.content}
                      </span>
                    </label>
                  ))}
                </fieldset>
              )}

              {selectablePlans.length > 0 ? (
                <p className="pt-2 text-sm font-medium">予定以外の学習</p>
              ) : null}

              <button
                type="button"
                onClick={() => {
                  setView("textbook");
                  setSelectedTarget("");
                  setError(null);
                }}
                className="flex min-h-20 w-full items-center gap-4 rounded-xl border p-4 text-left transition hover:border-primary/60 hover:bg-info/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="rounded-lg bg-info/15 p-2 text-primary">
                  <BookOpen className="h-5 w-5" aria-hidden="true" />
                </span>
                <span>
                  <span className="block font-semibold text-foreground">
                    参考書から選ぶ
                  </span>
                  <span className="mt-1 block text-sm text-muted-foreground">
                    登録済みの教材を指定
                  </span>
                </span>
              </button>

              <button
                type="button"
                onClick={() => {
                  setView("free");
                  setSelectedTarget("");
                  setError(null);
                }}
                className="flex min-h-20 w-full items-center gap-4 rounded-xl border p-4 text-left transition hover:border-primary/60 hover:bg-info/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="rounded-lg bg-muted p-2 text-foreground">
                  <PencilLine className="h-5 w-5" aria-hidden="true" />
                </span>
                <span>
                  <span className="block font-semibold text-foreground">
                    自由に入力する
                  </span>
                  <span className="mt-1 block text-sm text-muted-foreground">
                    復習、過去問、授業など
                  </span>
                </span>
              </button>
            </div>
          ) : null}

          {view === "textbook" ? (
            <div>
              <Label className="mb-2 block">参考書</Label>
              {textbooksPending ? (
                <div
                  className="rounded-lg bg-muted/50 p-4 text-sm text-muted-foreground"
                  role="status"
                >
                  参考書を読み込んでいます…
                </div>
              ) : textbooksError ? (
                <div className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                  <p>参考書を読み込めませんでした。</p>
                  <button
                    type="button"
                    onClick={() => refetchTextbooks()}
                    className="mt-2 font-medium underline"
                  >
                    再試行
                  </button>
                </div>
              ) : textbooks.length === 0 ? (
                <div className="rounded-lg bg-muted/50 p-4 text-sm text-muted-foreground">
                  <p>登録済みの参考書がありません。</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="mt-3"
                    autoFocus
                    onClick={() => setView("new-textbook")}
                  >
                    参考書を登録する
                  </Button>
                </div>
              ) : (
                <div className="space-y-2" aria-label="参考書を選択">
                  {textbooks.map((textbook, index) => {
                    const selected = manualTextbookId === textbook.id;
                    return (
                      <button
                        key={textbook.id}
                        type="button"
                        autoFocus={
                          manualTextbookId == null ? index === 0 : selected
                        }
                        aria-pressed={selected}
                        onClick={() => {
                          setManualTextbookId(textbook.id);
                          setError(null);
                        }}
                        className={`flex min-h-14 w-full items-center gap-3 rounded-lg border px-3 py-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${selected ? "border-primary bg-info/10" : "hover:border-muted-foreground"}`}
                      >
                        <span
                          className={`h-4 w-4 shrink-0 rounded-full border-2 ${selected ? "border-[5px] border-primary" : "border-border"}`}
                        />
                        <span className="min-w-0">
                          <span className="block truncate font-medium text-foreground">
                            {textbook.name}
                          </span>
                          <span className="mt-0.5 block text-xs text-muted-foreground">
                            {SUBJECTS.find(
                              (subject) => subject.value === textbook.subject
                            )?.label ?? "科目なし"}
                          </span>
                        </span>
                      </button>
                    );
                  })}
                  <button
                    type="button"
                    onClick={() => setView("new-textbook")}
                    className="mt-2 min-h-11 text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    ＋ 新しい参考書を登録
                  </button>
                </div>
              )}
            </div>
          ) : null}

          {view === "free" ? (
            <div className="space-y-5">
              <div>
                <Label htmlFor="session-activity">学習内容</Label>
                <Input
                  id="session-activity"
                  autoFocus
                  className="mt-2 h-11"
                  placeholder="例：英文法の復習"
                  maxLength={100}
                  value={manualLabel}
                  onChange={(event) => {
                    setManualLabel(event.target.value);
                    setError(null);
                  }}
                />
              </div>
              <div>
                <Label className="mb-2 block">科目（任意）</Label>
                <div className="flex flex-wrap gap-2">
                  {SUBJECTS.map((subject) => {
                    const selected = manualSubject === subject.value;
                    return (
                      <button
                        key={subject.value}
                        type="button"
                        aria-pressed={selected}
                        onClick={() =>
                          setManualSubject(selected ? null : subject.value)
                        }
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
                <Label htmlFor="session-new-textbook-name">参考書名</Label>
                <Input
                  id="session-new-textbook-name"
                  autoFocus
                  className="mt-2"
                  value={newTextbookName}
                  onChange={(event) => {
                    setNewTextbookName(event.target.value);
                    setError(null);
                  }}
                  placeholder="例：青チャートIA"
                  maxLength={100}
                />
              </div>
              <div>
                <Label className="mb-2 block">科目（任意）</Label>
                <div className="flex flex-wrap gap-2">
                  {SUBJECTS.map((subject) => {
                    const selected = newTextbookSubject === subject.value;
                    return (
                      <button
                        key={subject.value}
                        type="button"
                        aria-pressed={selected}
                        onClick={() =>
                          setNewTextbookSubject(selected ? null : subject.value)
                        }
                        className="min-h-10 rounded-full border px-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
              <div>
                <Label htmlFor="session-new-textbook-unit">範囲の単位</Label>
                <select
                  id="session-new-textbook-unit"
                  value={newTextbookUnit}
                  onChange={(event) =>
                    setNewTextbookUnit(
                      event.target
                        .value as (typeof RANGE_UNITS)[number]["value"]
                    )
                  }
                  className="mt-2 h-10 w-full rounded-lg border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {RANGE_UNITS.map((unit) => (
                    <option key={unit.value} value={unit.value}>
                      {unit.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          ) : null}

          {error ? (
            <div
              role="alert"
              className="mt-4 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive"
            >
              {error}
            </div>
          ) : null}
        </div>

        <div className="flex flex-col gap-2 border-t bg-card px-5 pt-4 pb-[calc(1rem+env(safe-area-inset-bottom))] sm:flex-row sm:items-center sm:justify-between sm:pb-4">
          {view === "new-textbook" ? (
            <Button
              type="button"
              className="min-h-11 w-full"
              onClick={addNewTextbook}
              disabled={createTextbook.isPending}
            >
              {createTextbook.isPending ? "登録中…" : "参考書を登録して選択"}
            </Button>
          ) : (
            <>
              <Button
                type="button"
                variant="ghost"
                className="h-11 w-full sm:w-auto"
                onClick={onManualLog}
              >
                あとから記録
              </Button>
              {(selectedTarget.startsWith("plan:") ||
                (view === "textbook" && manualTextbookId != null) ||
                view === "free") && (
                <Button
                  type="button"
                  className="h-11 w-full sm:w-auto"
                  onClick={start}
                >
                  <Play aria-hidden="true" />
                  計測を開始
                </Button>
              )}
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
