"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  BookOpen,
  Pause,
  PencilLine,
  Play,
  Square,
} from "lucide-react";
import { notifyDemoReadOnly } from "@/lib/demo-client";
import { trackEvent } from "@/lib/analytics";
import { toast } from "sonner";
import QuickManualStudyLogDialog from "@/app/components/QuickManualStudyLogDialog";
import {
  NumberStepper,
  RangeUnitSelect,
} from "@/app/components/StudyFields";
import { studyLogsKey } from "@/app/hooks/useStudyLogs";
import {
  studyPlansKey,
  useStudyPlans,
  type StudyPlan,
} from "@/app/hooks/useStudyPlans";
import {
  useCreateTextbook,
  useTextbooks,
} from "@/app/hooks/useTextbooks";
import { todayYmd } from "@/lib/date";
import { studyPlanLabel } from "@/lib/studyPlan";
import { SUBJECTS, subjectColor } from "@/lib/subjects";
import {
  type ActiveStudySession,
  elapsedStudyMs,
  formatStudyElapsed,
  parseStoredStudySession,
  pauseStudySession,
  recordedMinutes,
  resumeStudySession,
  reviewStudySession,
  startStudySession,
  studySessionStorageKey,
} from "@/lib/studySession";
import { RANGE_UNITS } from "@/lib/validations/studyPlan";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const responseError = async (response: Response) => {
  const body = await response.json().catch(() => null);
  if (typeof body?.error === "string") return body.error;
  if (Array.isArray(body?.error) && typeof body.error[0]?.message === "string") {
    return body.error[0].message;
  }
  return "実績を保存できませんでした";
};

type PickerView = "choose" | "textbook" | "free" | "new-textbook";

export default function StudySessionManager({
  initialPlans,
  userId,
  readOnly = false,
  variant = "compact",
}: {
  initialPlans: StudyPlan[];
  userId: string;
  readOnly?: boolean;
  /** hero: ログイン直後の集中スタート画面向けに、ボタンを中央・大きく表示する */
  variant?: "compact" | "hero";
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const {
    data: textbooks = [],
    isPending: textbooksPending,
    isError: textbooksError,
    refetch: refetchTextbooks,
  } = useTextbooks();
  const createTextbook = useCreateTextbook();
  const { data: studyPlans = [] } = useStudyPlans(initialPlans);
  const plans = useMemo(
    () =>
      studyPlans
        .filter((plan) => plan.date.slice(0, 10) === todayYmd())
        .map((plan) => ({
          id: plan.id,
          content: studyPlanLabel(plan),
          done: plan.done,
          subject: plan.subject,
          textbookId: plan.textbookId,
          textbookName: plan.textbook?.name ?? null,
          rangeStart: plan.rangeStart,
          rangeEnd: plan.rangeEnd,
          rangeUnit: plan.rangeUnit,
          recordedMinutes: plan.studyLogId == null ? null : 0,
        })),
    [studyPlans]
  );
  const storageKey = studySessionStorageKey(userId);
  const selectablePlans = useMemo(
    () => plans.filter((plan) => plan.recordedMinutes == null),
    [plans]
  );

  const [hydrated, setHydrated] = useState(false);
  const [session, setSession] = useState<ActiveStudySession | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerView, setPickerView] = useState<PickerView>("choose");
  const [pickerError, setPickerError] = useState<string | null>(null);
  const [manualLogOpen, setManualLogOpen] = useState(false);
  const [selectedTarget, setSelectedTarget] = useState("");
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
  const [minutes, setMinutes] = useState(1);
  const [rangeStart, setRangeStart] = useState<number | null>(null);
  const [rangeEnd, setRangeEnd] = useState<number | null>(null);
  const [rangeUnit, setRangeUnit] = useState<string | null>(null);
  const [memo, setMemo] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [confirmDiscard, setConfirmDiscard] = useState(false);

  useEffect(() => {
    const restoreTimer = window.setTimeout(() => {
      const storedValue = window.localStorage.getItem(storageKey);
      const restored = parseStoredStudySession(storedValue);
      if (!restored && storedValue) {
        window.localStorage.removeItem(storageKey);
        toast.error("以前のタイマー情報を復元できなかったため、リセットしました");
      }
      setSession(restored);
      if (restored?.status === "reviewing") {
        setMinutes(recordedMinutes(restored));
        setRangeStart(restored.rangeStart);
        setRangeEnd(restored.rangeEnd);
        setRangeUnit(restored.rangeUnit);
      }
      setHydrated(true);
    }, 0);

    const sync = (event: StorageEvent) => {
      if (event.key === storageKey) {
        const synced = parseStoredStudySession(event.newValue);
        setSession(synced);
        if (synced?.status === "reviewing") {
          setMinutes(recordedMinutes(synced));
          setRangeStart(synced.rangeStart);
          setRangeEnd(synced.rangeEnd);
          setRangeUnit(synced.rangeUnit);
        }
      }
    };
    window.addEventListener("storage", sync);
    return () => {
      window.clearTimeout(restoreTimer);
      window.removeEventListener("storage", sync);
    };
  }, [storageKey]);

  useEffect(() => {
    if (!hydrated) return;
    if (session) {
      window.localStorage.setItem(storageKey, JSON.stringify(session));
    } else {
      window.localStorage.removeItem(storageKey);
    }
  }, [hydrated, session, storageKey]);

  useEffect(() => {
    if (session?.status !== "running") return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [session?.status]);

  const openPicker = () => {
    setPickerView("choose");
    setPickerError(null);
    setSelectedTarget(
      selectablePlans[0] ? `plan:${selectablePlans[0].id}` : ""
    );
    setManualLabel("");
    setManualSubject(null);
    setManualTextbookId(null);
    setNewTextbookName("");
    setNewTextbookSubject(null);
    setNewTextbookUnit("page");
    setPickerOpen(true);
  };

  const addNewTextbook = () => {
    const name = newTextbookName.trim();
    if (!name) {
      setPickerError("参考書名を入力してください");
      return;
    }
    setPickerError(null);
    createTextbook.mutate(
      { name, subject: newTextbookSubject, rangeUnit: newTextbookUnit },
      {
        onSuccess: (textbook) => {
          setManualTextbookId(textbook.id);
          setNewTextbookName("");
          setPickerView("textbook");
        },
        onError: (error) => setPickerError(error.message),
      }
    );
  };

  const beginSession = () => {
    const timestamp = Date.now();
    if (selectedTarget.startsWith("plan:")) {
      const planId = Number(selectedTarget.slice("plan:".length));
      const plan = selectablePlans.find((candidate) => candidate.id === planId);
      if (!plan) return;
      setSession(
        startStudySession(
          {
            planId: plan.id,
            label: plan.content,
            subject: plan.subject,
            textbookId: plan.textbookId,
            rangeStart: plan.rangeStart,
            rangeEnd: plan.rangeEnd,
            rangeUnit: plan.rangeUnit,
          },
          timestamp
        )
      );
    } else if (pickerView === "textbook") {
      const textbook = textbooks.find((item) => item.id === manualTextbookId);
      if (!textbook) {
        setPickerError("参考書を選んでください");
        return;
      }
      setSession(
        startStudySession(
          {
            planId: null,
            label: textbook.name,
            subject: textbook.subject,
            textbookId: textbook.id,
            rangeStart: null,
            rangeEnd: null,
            rangeUnit: textbook?.rangeUnit ?? null,
          },
          timestamp
        )
      );
    } else if (pickerView === "free") {
      const trimmedLabel = manualLabel.trim();
      if (!trimmedLabel) {
        setPickerError("勉強する内容を入力してください");
        return;
      }
      setSession(
        startStudySession(
          {
            planId: null,
            label: trimmedLabel,
            subject: manualSubject,
            textbookId: null,
            rangeStart: null,
            rangeEnd: null,
            rangeUnit: null,
          },
          timestamp
        )
      );
    } else {
      setPickerError("勉強する内容を選んでください");
      return;
    }
    setNow(timestamp);
    setPickerOpen(false);
    toast.success("学習時間の計測を開始しました");
  };

  const moveToReview = () => {
    if (!session) return;
    const reviewed = reviewStudySession(session);
    setSession(reviewed);
    setMinutes(recordedMinutes(reviewed));
    setRangeStart(reviewed.rangeStart);
    setRangeEnd(reviewed.rangeEnd);
    setRangeUnit(reviewed.rangeUnit);
    // 「その他の学習」で自由入力した内容（＝ラベル）をメモの初期値に引き継ぎ、
    // 記録に「何をやったか」を残せるようにする。予定・参考書由来のラベルは対象外。
    const freeTextLabel =
      reviewed.planId == null &&
      reviewed.textbookId == null &&
      reviewed.label !== "その他の学習"
        ? reviewed.label
        : "";
    setMemo(freeTextLabel);
    setSaveError(null);
    setConfirmDiscard(false);
  };

  const save = async (asManual = false) => {
    if (!session) return;
    setSaving(true);
    setSaveError(null);
    const planId = asManual ? null : session.planId;
    try {
      const response = await fetch(
        planId == null
          ? "/api/study-logs"
          : `/api/study-plans/${planId}/complete`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(
            planId == null
              ? {
                  date: todayYmd(),
                  minutes,
                  subject: session.subject,
                  textbookId: session.textbookId,
                  rangeStart,
                  rangeEnd,
                  rangeUnit,
                  memo,
                }
              : { minutes, rangeStart, rangeEnd, rangeUnit, memo }
          ),
        }
      );
      if (!response.ok) {
        setSaveError(await responseError(response));
        return;
      }

      const result = (await response.json()) as { isFirstStudyLog?: boolean };
      trackEvent(
        result.isFirstStudyLog
          ? "first_study_log_created"
          : "study_log_created",
        { record_method: planId == null ? "timer" : "plan" }
      );

      clearSession();
      setConfirmDiscard(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: studyLogsKey }),
        queryClient.invalidateQueries({ queryKey: studyPlansKey }),
      ]);
      router.refresh();
      toast.success(
        planId == null
          ? "学習実績を保存しました"
          : "実績を保存し、予定を完了しました"
      );
    } catch {
      setSaveError("通信に失敗しました。接続を確認して、もう一度お試しください");
    } finally {
      setSaving(false);
    }
  };

  const elapsed = session ? elapsedStudyMs(session, now) : 0;

  const clearSession = () => {
    window.localStorage.removeItem(storageKey);
    setSession(null);
  };

  const isHero = variant === "hero";

  return (
    <div
      className={
        isHero
          ? "flex w-full flex-col items-center gap-3"
          : "flex w-full flex-col items-end gap-2"
      }
    >
      {!hydrated ? (
        <Button
          type="button"
          className={
            isHero ? "h-14 px-8 text-lg" : "h-11 w-full sm:w-auto"
          }
          disabled
        >
          タイマーを確認中…
        </Button>
      ) : session ? null : (
        <Button
          type="button"
          className={
            isHero ? "h-14 px-8 text-lg" : "h-11 w-full sm:w-auto"
          }
          title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
          onClick={readOnly ? notifyDemoReadOnly : openPicker}
        >
          <Play aria-hidden="true" />
          学習を始める
        </Button>
      )}

      {readOnly && !session && (
        <p
          className={
            isHero
              ? "text-xs text-muted-foreground"
              : "text-xs text-muted-foreground sm:text-right"
          }
        >
          デモアカウントでは計測できません
        </p>
      )}

      {session && session.status !== "reviewing" && (
        <div className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-8 bg-background px-6 text-center">
          <div className="flex flex-col items-center gap-3">
            <p className="text-sm font-medium text-primary">
              {session.status === "running" ? "● 計測中" : "一時停止中"}
            </p>
            <p className="max-w-md truncate text-lg font-medium">
              {session.label}
            </p>
            <p className="font-mono text-6xl font-bold tabular-nums sm:text-7xl">
              {formatStudyElapsed(elapsed)}
            </p>
          </div>
          <div className="flex w-full max-w-xs flex-col gap-3 sm:max-w-none sm:flex-row sm:justify-center">
            {session.status === "running" ? (
              <Button
                type="button"
                variant="outline"
                className="h-12 sm:w-40"
                onClick={() => setSession(pauseStudySession(session))}
              >
                <Pause aria-hidden="true" />
                一時停止
              </Button>
            ) : (
              <Button
                type="button"
                className="h-12 sm:w-40"
                onClick={() => setSession(resumeStudySession(session))}
              >
                <Play aria-hidden="true" />
                再開
              </Button>
            )}
            <Button
              type="button"
              className="h-12 sm:w-40"
              onClick={moveToReview}
            >
              <Square aria-hidden="true" />
              学習を終了
            </Button>
          </div>
        </div>
      )}

      <Dialog open={pickerOpen} onOpenChange={setPickerOpen}>
        <DialogContent className="top-auto bottom-0 flex max-h-[90dvh] max-w-none translate-y-0 flex-col gap-0 overflow-hidden rounded-b-none p-0 sm:top-1/2 sm:bottom-auto sm:max-w-md sm:-translate-y-1/2 sm:rounded-xl">
          <DialogHeader className="border-b px-5 py-4 pr-12">
            {pickerView !== "choose" ? (
              <button
                type="button"
                onClick={() => {
                  setPickerError(null);
                  if (pickerView === "new-textbook") {
                    setPickerView("textbook");
                  } else {
                    setPickerView("choose");
                    setSelectedTarget(
                      selectablePlans[0]
                        ? `plan:${selectablePlans[0].id}`
                        : ""
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
              {pickerView === "textbook"
                ? "参考書から選ぶ"
                : pickerView === "free"
                  ? "自由に入力する"
                  : pickerView === "new-textbook"
                    ? "新しい参考書を登録"
                    : "何を勉強しますか？"}
            </DialogTitle>
            <DialogDescription>
              {pickerView === "choose"
                ? "今日の予定を選ぶか、学習内容を選びます。"
                : pickerView === "new-textbook"
                  ? "登録後、その参考書を選んだ状態に戻ります。"
                  : "勉強する内容を決めて、時間計測を始めます。"}
            </DialogDescription>
          </DialogHeader>

          <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
            {pickerView === "choose" ? (
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
                    setPickerView("textbook");
                    setSelectedTarget("");
                    setPickerError(null);
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
                    setPickerView("free");
                    setSelectedTarget("");
                    setPickerError(null);
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

            {pickerView === "textbook" ? (
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
                      onClick={() => setPickerView("new-textbook")}
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
                            setPickerError(null);
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
                      onClick={() => setPickerView("new-textbook")}
                      className="mt-2 min-h-11 text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      ＋ 新しい参考書を登録
                    </button>
                  </div>
                )}
              </div>
            ) : null}

            {pickerView === "free" ? (
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
                      setPickerError(null);
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

            {pickerView === "new-textbook" ? (
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
                      setPickerError(null);
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
                            setNewTextbookSubject(
                              selected ? null : subject.value
                            )
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
                  <Label htmlFor="session-new-textbook-unit">
                    範囲の単位
                  </Label>
                  <select
                    id="session-new-textbook-unit"
                    value={newTextbookUnit}
                    onChange={(event) =>
                      setNewTextbookUnit(
                        event.target.value as (typeof RANGE_UNITS)[number]["value"]
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

            {pickerError ? (
              <div
                role="alert"
                className="mt-4 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive"
              >
                {pickerError}
              </div>
            ) : null}
          </div>

          <div className="flex flex-col gap-2 border-t bg-card px-5 pt-4 pb-[calc(1rem+env(safe-area-inset-bottom))] sm:flex-row sm:items-center sm:justify-between sm:pb-4">
            {pickerView === "new-textbook" ? (
              <Button
                type="button"
                className="min-h-11 w-full"
                onClick={addNewTextbook}
                disabled={createTextbook.isPending}
              >
                {createTextbook.isPending
                  ? "登録中…"
                  : "参考書を登録して選択"}
              </Button>
            ) : (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  className="h-11 w-full sm:w-auto"
                  onClick={() => {
                    setPickerOpen(false);
                    setManualLogOpen(true);
                  }}
                >
                  あとから記録
                </Button>
                {(selectedTarget.startsWith("plan:") ||
                  (pickerView === "textbook" &&
                    manualTextbookId != null) ||
                  pickerView === "free") && (
                  <Button
                    type="button"
                    className="h-11 w-full sm:w-auto"
                    onClick={beginSession}
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

      <Dialog
        open={session?.status === "reviewing"}
        onOpenChange={(open) => {
          if (!open && session && !saving) {
            setSession({ ...session, status: "paused" });
          }
        }}
      >
        <DialogContent
          showCloseButton={!saving}
          className="top-auto bottom-0 max-h-[90dvh] translate-y-0 overflow-y-auto rounded-b-none sm:top-1/2 sm:bottom-auto sm:max-w-lg sm:-translate-y-1/2 sm:rounded-xl"
        >
          <DialogHeader>
            <DialogTitle>おつかれさまでした</DialogTitle>
            <DialogDescription>
              計測した時間と実施内容を確認して、実績を保存します。
            </DialogDescription>
          </DialogHeader>

          {session && (
            <div className="space-y-4">
              <div className="rounded-lg bg-muted p-3">
                <p className="font-medium">{session.label}</p>
                <p className="mt-1 font-mono text-2xl font-bold tabular-nums">
                  {formatStudyElapsed(elapsedStudyMs(session))}
                </p>
              </div>

              {saveError && (
                <div role="alert" className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                  <p>{saveError}</p>
                  {session.planId != null && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="mt-2"
                      disabled={saving}
                      onClick={() => save(true)}
                    >
                      その他の実績として保存
                    </Button>
                  )}
                </div>
              )}

              <div className="space-y-1">
                <Label htmlFor="session-minutes">学習時間（分）</Label>
                <Input
                  id="session-minutes"
                  type="number"
                  min={1}
                  max={1440}
                  inputMode="numeric"
                  className="h-11 w-28 text-base"
                  value={minutes}
                  onChange={(event) => setMinutes(Number(event.target.value))}
                />
                {elapsedStudyMs(session) < 60_000 && (
                  <p className="text-xs text-muted-foreground">
                    1分未満のため、1分として記録します。
                  </p>
                )}
              </div>

              <div className="space-y-2">
                <Label>実施範囲（任意）</Label>
                <div className="flex flex-wrap items-start gap-2">
                  <NumberStepper
                    ariaLabel="実施範囲の開始"
                    placeholder="開始"
                    value={rangeStart}
                    onChange={setRangeStart}
                  />
                  <span className="pt-2 text-muted-foreground">〜</span>
                  <NumberStepper
                    ariaLabel="実施範囲の終了"
                    placeholder="終了"
                    value={rangeEnd}
                    onChange={setRangeEnd}
                  />
                  <RangeUnitSelect
                    ariaLabel="範囲の単位"
                    value={rangeUnit}
                    onChange={setRangeUnit}
                  />
                </div>
              </div>

              <div className="space-y-1">
                <Label htmlFor="session-memo">メモ（任意）</Label>
                <Input
                  id="session-memo"
                  value={memo}
                  maxLength={500}
                  onChange={(event) => setMemo(event.target.value)}
                />
              </div>

              {confirmDiscard && (
                <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3">
                  <p className="text-sm">計測結果を保存せずに破棄しますか？</p>
                  <div className="mt-2 flex gap-2">
                    <Button type="button" variant="outline" size="sm" onClick={() => setConfirmDiscard(false)}>
                      戻る
                    </Button>
                    <Button type="button" variant="destructive" size="sm" onClick={clearSession}>
                      破棄する
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}

          <DialogFooter className="gap-2">
            <Button
              type="button"
              variant="ghost"
              disabled={saving}
              onClick={() => setConfirmDiscard(true)}
            >
              保存せず終了
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => session && setSession({ ...session, status: "paused" })}
            >
              タイマーへ戻る
            </Button>
            <Button
              type="button"
              disabled={saving || minutes < 1 || minutes > 1440}
              onClick={() => save(false)}
            >
              {saving ? "保存中…" : "実績を保存"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <QuickManualStudyLogDialog
        open={manualLogOpen}
        onOpenChange={setManualLogOpen}
        onBack={() => {
          setManualLogOpen(false);
          setPickerOpen(true);
        }}
      />
    </div>
  );
}
